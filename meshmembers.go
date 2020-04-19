// Program meshmembers is a thin wrapper around HashiCorp's memberlist
package main

/*
 * meshmembers.go
 * Thin wrapper around HashiCorp's memberlist
 * By J. Stuart McMurray
 * Created 20200416
 * Last Modified 20200418
 */

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/memberlist"
)

var (
	/* SharedSecret is the secret shared amongst mesh members */
	SharedSecret = "i_used_the_default_from_github"
)

const (
	/* udpBufferSize is the size of UDP packets we'll send.  This allows
	for a much smaller MTU */
	udpBufferSize = 1024

	/* extAddrURL is the URL to query to get our external address */
	extAddrURL = "https://icanhazip.com"
)

func main() {
	var (
		sockPath = flag.String(
			"socket",
			"",
			"Unix socket `path` for listing members",
		)
		removeSockFirst = flag.Bool(
			"remove-existing-socket",
			false,
			"Remove the unix socket file before listening",
		)
		nodeName = flag.String(
			"name",
			defaultNodeName(),
			fmt.Sprintf(
				"Node `name`",
			),
		)
		listenAddr = flag.String(
			"listen",
			"0.0.0.0:7887",
			"Listen `address` and port",
		)
		extAddr = flag.String(
			"external",
			"",
			"External IP `address` which will be found by "+
				"querying icanhazip if unset",
		)
		password = flag.String(
			"secret",
			SharedSecret,
			"Mesh shared `secret`",
		)
		peers = flag.String(
			"peers",
			"",
			"Comma-separated `list` of known mesh members",
		)
		reportInterval = flag.Duration(
			"report-every",
			time.Hour,
			"Mesh size report `interval`",
		)
	)
	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			`Usage: %s [options]

Joins a mesh network and sends a list of members to anything connecting to its
socket.  By default the node's name will be composed of the platform, a MAC
address, and the time.

Options:
`,
			os.Args[0],
		)
		flag.PrintDefaults()
	}
	flag.Parse()

	/* Log to stdout, not stderr */
	log.SetOutput(os.Stdout)

	/* Figure out our listen address and port */
	ea, la, port, err := resolveAddresses(*listenAddr, *extAddr)
	if nil != err {
		log.Fatalf("Error resolving addresses: %v", err)
	}
	if "" == ea {
		ea = la
	}
	if "" == la {
		log.Printf("Listening on all interfaces")
	} else {
		log.Printf("Listen address: %s", la)
	}
	log.Printf("External address: %s", ea)
	log.Printf("Port: %d", port)

	/* Encryption key */
	key := sha256.Sum256([]byte(*password))

	/* Mesh config */
	nech := make(chan memberlist.NodeEvent)
	conf := memberlist.DefaultWANConfig()
	/* The above config's timings seem reasonable, but there's a few
	defaults not suitable for us. */
	conf.Name = *nodeName
	conf.BindAddr = la
	conf.BindPort = port
	conf.AdvertiseAddr = ea
	conf.AdvertisePort = port
	conf.GossipVerifyIncoming = true
	conf.GossipVerifyOutgoing = true
	conf.ProtocolVersion = memberlist.ProtocolVersionMax
	conf.SecretKey = key[:]
	conf.UDPBufferSize = udpBufferSize
	conf.Events = &memberlist.ChannelEventDelegate{Ch: nech}
	conf.Conflict = ConflictHandler{}
	conf.LogOutput = ioutil.Discard

	/* Handle events from the mesh */
	go HandleEvents(conf.Name, nech)

	/* Start our own node */
	log.Printf("Starting mesh listeners")
	m, err := memberlist.Create(conf)
	if nil != err {
		log.Fatalf("Error creating local node: %v", err)
	}
	log.Printf("This node: %s", FormatNode(m.LocalNode()))

	/* Listen for unix clients */
	if "" != *sockPath {
		ListenForClients(*sockPath, *removeSockFirst, m)
	}

	/* If we've peers to connect to, connect to them */
	if "" != *peers {
		n, err := connectToPeers(m, *peers)
		if nil != err {
			log.Printf(
				"Error connecting to initial peers: %v",
				err,
			)
		} else if 1 == n {
			log.Printf("Connected to 1 initial peer")
		} else {
			log.Printf("Connected to %d initial peers", n)
		}
	}

	/* Every so often print how many are in the mesh */
	for range time.Tick(*reportInterval) {
		log.Printf("Current mesh size: %d", m.NumMembers())
	}
}

/* connectToPeers tries to connect m to the peers in the comma-separated list
csl which should contain host:port pairs.  It only returns if no peers were
contacted. */
func connectToPeers(m *memberlist.Memberlist, csl string) (int, error) {
	/* Clean up the list of peers */
	ps := strings.Split(csl, ",")
	last := 0
	for _, p := range ps {
		p = strings.TrimSpace(p)
		if "" == p {
			continue
		}
		ps[last] = p
		last++
	}
	ps = ps[:last]
	if 0 == len(ps) {
		return 0, errors.New("no usable peers in list")
	}

	/* Join with existing peers */
	log.Printf("Initial peer list: %s", ps)
	n, err := m.Join(ps)
	if nil != err {
		return 0, fmt.Errorf("error joining mesh: %w", err)
	}
	return n, nil
}

/* defaultNodeName returns a name composed of the platform, MAC address, and
time */
func defaultNodeName() string {
	nifs, err := net.Interfaces()
	if nil != err {
		log.Fatalf("Interfaces: %v", err)
	}
	var hwaddrs []string
	for _, nif := range nifs {
		/* Don't want loopback interfaces */
		if 0 != nif.Flags&net.FlagLoopback {
			continue
		}
		/* Don't want interfaces with no hardware address */
		a := nif.HardwareAddr.String()
		if "" == a {
			continue
		}
		hwaddrs = append(hwaddrs, a)
	}

	/* Get the first one */
	sort.Strings(hwaddrs)

	/* If we haven't a MAC address, it's a bit weird but not a problem */
	if 0 == len(hwaddrs) {
		hwaddrs = append(hwaddrs, "unknown")
	}

	return fmt.Sprintf(
		"%s-%s-%s-%s",
		runtime.GOOS,
		runtime.GOARCH,
		hwaddrs[0],
		strconv.FormatInt(time.Now().UnixNano(), 36),
	)
}

/* resolveAddresses makes sure we have a listen address and port and tries to
get our external address */
func resolveAddresses(
	la string,
	ea string,
) (extAddr, listenAddr string, port int, err error) {
	/* Work out the listen address */
	if "" == la {
		return "", "", 0, fmt.Errorf("no listen address specified")
	}
	var p string
	listenAddr, p, err = net.SplitHostPort(la)
	if nil != err {
		return "", "", 0, fmt.Errorf("parsing address %q: %w", ea, err)
	}
	port, err = strconv.Atoi(p)
	if nil != err {
		return "", "", 0, fmt.Errorf("paring port %q: %w", p, err)
	}

	/* If we have an external address already, use it */
	if "" != ea {
		extAddr = ea
		return
	}

	/* Try to get our external address */
	res, err := http.Get(extAddrURL)
	if nil != err {
		/* We tried */
		log.Printf("Error querying %q: %v", extAddrURL, err)
		return
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if nil != err {
		log.Printf("Error reading reply from %q: %v", extAddrURL, err)
		return
	}

	/* Got an answer, maybe it's an address? */
	ip := net.ParseIP(strings.TrimSpace(string(b)))
	if nil == ip {
		log.Printf("Unable to parse reply %q from %q", b, extAddrURL)
		return
	}
	extAddr = ip.String()

	return
}
