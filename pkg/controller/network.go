package controller

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	blockchain "github.com/p9c/pod/pkg/chain"
	"github.com/p9c/pod/pkg/fec"
	"github.com/p9c/pod/pkg/log"
	"io"
	"net"
	"time"
)

const (
	// MaxDatagramSize is the largest a packet could be,
	// it is a little larger but this is easier to calculate.
	// There is only one listening thread but it needs a buffer this size for
	// worst case largest block possible.
	// Note also this is why FEC is used on the packets in case some get lost it
	// has to puncture 6 of the 9.
	// This protocol is connectionless and stateless so if one misses,
	// the next one probably won't, usually a second or 3 later
	MaxDatagramSize      = blockchain.MaxBlockBaseSize / 3
	UDP6MulticastAddress = "ff02::1"
	UDP4MulticastAddress = "224.0.0.1"
)

var (
	MCAddresses = []*net.UDPAddr{
		{IP: net.ParseIP(UDP6MulticastAddress), Port: 11049},
		{IP: net.ParseIP(UDP4MulticastAddress), Port: 11049},
	}
)

// Send broadcasts bytes on the given multicast address with each shard
// labeled with a random 32 bit nonce to identify its group to the listener's
// handler function
func Send(addr *net.UDPAddr, by []byte, magic [4]byte,
	ciph cipher.AEAD, conn *net.UDPConn) (shards [][]byte, err error) {
	nonce := make([]byte, ciph.NonceSize())
	//log.DEBUG(len(nonce))
	var bb []byte
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		log.ERROR(err)
		return
	}
	if ciph != nil {
		bb = ciph.Seal(nil, nonce, by, nil)
	}
	//log.SPEW(bb)
	//bb = by
	shards, err = fec.Encode(bb)
	if err != nil {
		return
	}
	//log.SPEW(shards)
	// nonce is a batch identifier for the FEC encoded shards which are sent
	// out as individual packets
	prefix := append(nonce, magic[:]...)
	if err != nil {
		return
	}
	var n, cumulative int
	for i := range shards {
		shards[i] = append(prefix, shards[i]...)
		n, err = conn.WriteToUDP(shards[i], addr)
		if err != nil {
			log.ERROR(err, len(shards[i]))
			return
		}
		cumulative += n
	}
	log.TRACE("wrote", cumulative, "by to multicast address", addr.IP,
		"port",
		addr.Port)
	return
}

func SendShards(addr *net.UDPAddr, shards [][]byte, conn *net.UDPConn) (err error) {
	var n, cumulative int
	for i := range shards {
		n, err = conn.WriteToUDP(shards[i], addr)
		if err != nil {
			log.ERROR(err, len(shards[i]))
			return
		}
		cumulative += n
	}
	fmt.Printf("resent %v bytes to multicast address %v port %v %v\r",
		cumulative, addr.IP, addr.Port, time.Now())
	return
}

// Listen binds to the UDP address and port given and writes packets received
// from that address to a buffer which is passed to a handler
func Listen(address *net.UDPAddr, handler func(*net.UDPAddr, int,
	[]byte)) (cancel context.CancelFunc, err error) {
	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background())
	log.DEBUG("resolving", address)
	//addr, err := net.ResolveUDPAddr("udp", address)
	//if err != nil {
	//	log.ERROR(err)
	//	cancel()
	//	return
	//}
	//log.DEBUG("resolved", addr.IP, addr.Uint16, addr.String())
	var conn *net.UDPConn
	conn, err = net.ListenUDP("udp", address)
	if err != nil {
		log.ERROR(err)
		cancel()
		return
	}

	err = conn.SetReadBuffer(MaxDatagramSize)
	if err != nil {
		log.ERROR(err)
	}
	buffer := make([]byte, MaxDatagramSize)
	go func() {
	out:
		// read from socket until context is cancelled
		for {
			numBytes, src, err := conn.ReadFromUDP(buffer)
			if err != nil {
				log.ERROR("ReadFromUDP failed:", err)
				continue
			}
			handler(src, numBytes, buffer)
			select {
			case <-ctx.Done():
				break out
			default:
			}
		}
	}()
	return
}
