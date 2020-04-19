package main

/*
 * client.go
 * Handle local clients
 * By J. Stuart McMurray
 * Created 20200417
 * Last Modified 20200418
 */

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
)

const (
	/* acceptWait is the wait after a temporary accept failure */
	acceptWait = time.Second

	/* readWait is the time to wait after a 0-byte read or
	non-disconnectworthy read error */
	readWait = time.Second

	/* maxClients is the maximum number of simultaneous clients we allow,
	though nofiles ulimit might be lower. */
	maxClients = 1024
)

/* localClient holds a local client's conn and tag */
type localClient struct {
	tag string
	c   *net.UnixConn
}

var (
	/* clients holds the list of connected clients, for broadcasting */
	clients  = make([]*localClient, maxClients)
	clientsL sync.Mutex

	/* clientCount counts the number of local clients we've had */
	clientCount  uint64
	clientCountL sync.Mutex
)

// ListenForClients listens for and handles local clients.  If rm is true the
// path will be removed before listening.  On return clients can connect.
// ListenForClients terminates the program on error.
func ListenForClients(path string, rm bool, m *memberlist.Memberlist) {
	/* Listen on the unix socket */
	if rm {
		if err := os.RemoveAll(path); nil != err {
			log.Fatalf("Error removing %s: %v", path, err)
		}
	}
	ul, err := ListenUnix(path)
	if nil != err {
		log.Fatalf("Unable to listen on %s: %s", path, err)
	}
	log.Printf("Listening for local clients on %s", ul.Addr())
	go handleClients(ul, m)
}

// ListenUnix listens on a unix Socket
func ListenUnix(path string) (*net.UnixListener, error) {
	/* Make sure the path is a path */
	ua, err := net.ResolveUnixAddr("unix", path)
	if nil != err {
		return nil, fmt.Errorf("resolving %s: %w", path, err)
	}
	/* Listen */
	l, err := net.ListenUnix("unix", ua)
	if nil != err {
		return nil, fmt.Errorf("listening on %s: %w", ua, err)
	}
	/* Unlink the socket when we're done with it */
	l.SetUnlinkOnClose(true)

	return l, nil
}

/* handleClients accepts and handles clients */
func handleClients(ul *net.UnixListener, m *memberlist.Memberlist) {
	for {
		/* Get a client */
		c, err := ul.AcceptUnix()
		if nil != err && !IsTemporary(err) {
			LeaveMeshAndExitWithError(fmt.Errorf(
				"acceping local client: %w",
				err,
			))
		}

		/* Add it to the list */
		go handleClient(c, m)
	}
}

/* handleClient sends the current state to the client and adds it to the list
to receive updates.  If there's no space in the list the client is told and
disconnected. */
func handleClient(c *net.UnixConn, m *memberlist.Memberlist) {
	/* Get the client's number */
	clientCountL.Lock()
	tag := fmt.Sprintf("client-%d", clientCount)
	clientCount++
	clientCountL.Unlock()
	log.Printf("[%s] Connected", tag)

	/* Roll a message with the state */
	var b bytes.Buffer
	ns := m.Members()
	fmt.Fprintf(&b, "Current nodes in mesh: %d\n", len(ns))
	for _, n := range ns {
		fmt.Fprintf(&b, "%s\n", FormatNode(n))
	}
	if _, err := c.Write(b.Bytes()); nil != err {
		log.Printf("[%s] Error sending member list: %v", tag, err)
		c.Close()
		return
	}

	/* Add to list of clients, for broadcasting */
	clientsL.Lock()
	defer clientsL.Unlock()

	/* Find an empty spot and stick it in */
	for i, p := range clients {
		if nil == p {
			/* Found a spot */
			clients[i] = &localClient{tag: tag, c: c}
			/* Wait for the client to disconnect, and remove it
			from the list when it does. */
			go waitForDisconnect(tag, i, c)
			return
		}
	}

	/* No empty space */
	fmt.Fprintf(c, "Too many connected clients, sorry\n")
	c.Close()
}

// LeaveMeshAndExitWithError tries to gracefully leave the mesh.  Either way,
// the program is terminated after printing the error.
func LeaveMeshAndExitWithError(err error) {
	/* TODO: Finish this */
	log.Fatalf("Fatal error: %s", err)
}

/* waitForDisconnect waits for the client to disconnect or have an error.  It
also reads and ignores bytes the client sends */
func waitForDisconnect(tag string, ci int, c net.Conn) {
	var (
		b     = make([]byte, 1)
		toErr interface{ Timeout() bool }
		n     int
		err   error
	)
	for {
		/* Block on read */
		n, err = c.Read(b)

		/* Ignore anything we're sent */
		if 0 != n {
			continue
		}

		/* If there's no real error, just wait and read again */
		if nil == err || IsTemporary(err) ||
			(errors.As(err, &toErr) && toErr.Timeout()) {
			time.Sleep(readWait)
			continue
		}

		/* Looks like there's an error */
		break
	}

	/* Client caused some sort of error, forget about and remove it */
	clients[ci].c.Close()
	clientsL.Lock()
	clients[ci] = nil
	clientsL.Unlock()

	/* Some errors aren't worth printing */
	if errors.Is(err, io.EOF) {
		log.Printf("[%s] Disconnected", tag)
		return
	}

	/* If we read on a closed connection (i.e. a write failed and we closed
	it elsewhere), don't log as it'll already be logged */
	/* TODO: Do above */
	log.Printf("[%s] Disconnected (%T): %v", tag, err, err)
}

// Broadcastf is like fmt.Printf but wraps Broadcast.  It makes sure the
// message ends in a newline */
func Broadcastf(f string, a ...interface{}) {
	m := fmt.Sprintf(f, a...)
	if !strings.HasSuffix(m, "\n") {
		m += "\n"
	}
	Broadcast([]byte(m))
}

// Broadcast sends b to all clients
func Broadcast(b []byte) {
	clientsL.Lock()
	defer clientsL.Unlock()

	/* Can't trust b won't change */
	wb := make([]byte, len(b))
	copy(wb, b)

	/* Send in parallel to everybody */
	for _, c := range clients {
		if nil == c {
			continue
		}
		go func(l *localClient) {
			/* Send this client the data */
			_, err := l.c.Write(wb)
			if nil == err {
				return
			}
			log.Printf("[%s] Write error: %v", l.tag, err)
			/* Something went wrong, lose the client */
			l.c.Close()
		}(c)
	}
}

// IsTemporary returns true if the error has a Temporary method which returns
// true.
func IsTemporary(err error) bool {
	var te interface{ Temporary() bool }
	if nil != err && errors.As(err, &te) && te.Temporary() {
		return true
	}
	return false
}
