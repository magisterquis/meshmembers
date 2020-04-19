package main

/*
 * event.go
 * Handle events from the mesh
 * By J. Stuart McMurray
 * Created 20200417
 * Last Modified 20200418
 */

import (
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/hashicorp/memberlist"
)

// ConflictHandler handles notifications that peer names conflict.  It
// implements memberlist.ConflictDelegate
type ConflictHandler struct{}

// NotifyConflict sends a message to clients that a new node has joined with
// the same name as an existing node.
func (c ConflictHandler) NotifyConflict(existing, other *memberlist.Node) {
	Broadcastf("[Name Conflict] Existing: %s New: %s", existing, other)
}

// HandleEvents handles events from the channel
func HandleEvents(ourName string, nech <-chan memberlist.NodeEvent) {
	for ne := range nech {
		go handleEvent(ourName, ne)
	}
}

/* handleEvent handles an event from the mesh */
func handleEvent(ourName string, ne memberlist.NodeEvent) {
	switch ne.Event {
	case memberlist.NodeJoin:
		/* Don't bother telling people we've joined */
		if ourName == ne.Node.Name {
			return
		}
		broadcastAndLogf("[Join] %s", FormatNode(ne.Node))
	case memberlist.NodeUpdate:
		broadcastAndLogf("[News] %s", FormatNode(ne.Node))
	case memberlist.NodeLeave:
		broadcastAndLogf("[Part] %s", FormatNode(ne.Node))
	default:
		broadcastAndLogf(
			"[Unknown event %v] %ss",
			ne.Event,
			FormatNode(ne.Node),
		)
	}
}

/* broadcastAndLogf logs and message and logs it as well */
func broadcastAndLogf(f string, a ...interface{}) {
	go Broadcastf(f, a...)
	log.Printf(f, a...)
}

// FormatNode formats a node as name (address:port)
func FormatNode(n *memberlist.Node) string {
	return fmt.Sprintf(
		"%s (%s)",
		n.Name,
		net.JoinHostPort(n.Addr.String(), strconv.Itoa(int(n.Port))),
	)
}
