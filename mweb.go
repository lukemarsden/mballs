// mweb is a program to demo multicast.
// Run it multiple times on different machines/containers and each
// instance will learn about the others through multicast.
// It will log to stdout every second the list of peers that it's seen.
// Flag --iface makes it use (and wait for) a particular interface (e.g. ethwe)
package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"
)

var (
	ipv4Addr = &net.UDPAddr{
		IP:   net.ParseIP("224.1.2.3"),
		Port: 7777,
	}
)

type PeerInfo struct {
	ID   int
	Name string
}

type Peer struct {
	info      PeerInfo
	addr      net.Addr
	lastHeard time.Time
}

var allPeers map[int]*Peer = make(map[int]*Peer)
var peersLock sync.Mutex

func printStatus() {
	peersLock.Lock()
	defer peersLock.Unlock()
	fmt.Printf("\n==========================================\n")
	fmt.Printf("           HERE ARE MY FRIENDS")
	fmt.Printf("\n==========================================\n")
	for _, p := range allPeers {
		fmt.Printf("     - %s %s\n", p.info.Name, p.addr)
	}
	fmt.Printf("==========================================\n")
}

func main() {
	var (
		ifaceName string
		err       error
	)
	flag.StringVar(&ifaceName, "iface", "", "name of interface for multicasting")
	flag.Parse()
	var iface *net.Interface = nil
	if ifaceName != "" {
		iface, err = EnsureInterface(ifaceName, 10)
		if err != nil {
			log.Fatal(err)
		}
	}

	rand.Seed(time.Now().Unix())
	myID := rand.Int()
	conn, _ := multicastListen(iface)
	go func() {
		m := make([]byte, 1024)
		for {
			n, addr, err := conn.ReadFrom(m)
			if err != nil {
				log.Fatal("multicast read:", err)
			}
			if n > 0 {
				decodeReceived(addr, m)
			}
		}
	}()

	sendconn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Fatal("send socket create:", err)
	}

	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			sendInfo(myID, sendconn)
			expirePeers()
			printStatus()
		}
	}
}

func sendInfo(id int, conn *net.UDPConn) {
	buf := new(bytes.Buffer)
	hostname, _ := os.Hostname()
	gob.NewEncoder(buf).Encode(PeerInfo{id, hostname})
	conn.WriteTo(buf.Bytes(), ipv4Addr)
}

func decodeReceived(addr net.Addr, buf []byte) {
	reader := bytes.NewReader(buf)
	decoder := gob.NewDecoder(reader)
	var info PeerInfo
	decoder.Decode(&info)
	peersLock.Lock()
	defer peersLock.Unlock()
	allPeers[info.ID] = &Peer{info, addr, time.Now()}
}

// Take out anyone we haven't heard from in a while
func expirePeers() {
	peersLock.Lock()
	defer peersLock.Unlock()
	for key, peer := range allPeers {
		if peer.lastHeard.Add(time.Second * 3).Before(time.Now()) {
			delete(allPeers, key)
		}
	}
}

func multicastListen(iface *net.Interface) (*net.UDPConn, error) {
	conn, err := net.ListenMulticastUDP("udp", iface, ipv4Addr)
	if err != nil {
		log.Fatal("multicast create:", err)
	}
	return conn, err
}

func EnsureInterface(ifaceName string, wait int) (iface *net.Interface, err error) {
	if iface, err = findInterface(ifaceName); err == nil || wait == 0 {
		return
	}
	for ; err != nil && wait > 0; wait -= 1 {
		time.Sleep(1 * time.Second)
		iface, err = findInterface(ifaceName)
	}
	return
}

func findInterface(ifaceName string) (iface *net.Interface, err error) {
	if iface, err = net.InterfaceByName(ifaceName); err != nil {
		return iface, fmt.Errorf("Unable to find interface %s", ifaceName)
	}
	if 0 == (net.FlagUp & iface.Flags) {
		return iface, fmt.Errorf("Interface %s is not up", ifaceName)
	}
	return
}
