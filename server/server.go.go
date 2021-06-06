// package server implements a udp tftp server
package server

import (
	"errors"
	"io"
	"log"
	"net"
	"time"

	pkt "github.com/muazrusli/ind-pro/packet"
)

// TftpMTftpMaxPacketSize is the practical limit of the size of a UDP
// packet, which is the size of an Ethernet MTU minus the headers of
// TFTP (4 bytes), UDP (8 bytes) and IP (20 bytes). (source: google).
const TftpMaxPacketSize = 1468

// AckTimeout is the total time to wait before timing out on an ACK.
var AckTimeout = time.Second * 20

// RetransmitTime is how long to wait before retransmitting a packet
// if an ACK has not yet been received.
var RetransmitTime = time.Second * 5

// ErrTimeout is returned when an action times out.
var ErrTimeout = errors.New("timed out")

// ErrUnexpectedPacket is returned when one packet type is
// received when a different one was expected.
var ErrUnexpectedPacket = errors.New("unexpected packet received")

// Function types for read and write abstraction
type ReaderFunc func(filename string) (r io.Reader, err error)
type WriterFunc func(filename string) (r io.Writer, err error)

// Server is a TFTP server.
type Server struct {
	// the directory to read and write files from.
	servdir string
	// functions for reading and writing
	ReadFunc  ReaderFunc
	WriteFunc WriterFunc
	// Set true to disable writes
	ReadOnly bool
}

// NewServer returns a new tftp Server instance that will
// serve files from the given directory
func NewServer(dir string, rf ReaderFunc, wr WriterFunc) *Server {
	return &Server{
		servdir:   dir,
		ReadFunc:  rf,
		WriteFunc: wr,
	}
}

// Handle a new client read or write request.
func (s *Server) HandleClient(addr *net.UDPAddr, req pkt.Packet) {
	log.Println("Handle Client!")

	reqpkt, ok := req.(*pkt.ReqPacket)
	if !ok {
		log.Printf("Invalid packet type for new connection!")
		return
	}
	// Re-resolve for verification
	clientaddr, err := net.ResolveUDPAddr("udp", addr.String())
	if err != nil {
		log.Printf("Error: %s", err)
		return
	}

	switch reqpkt.GetType() {
	case pkt.RRQ:
		err := s.HandleReadReq(reqpkt, clientaddr)
		if err != nil {
			log.Println("read request finished, with error:")
			log.Println(err)
		}
	case pkt.WRQ:
		err := s.HandleWriteReq(reqpkt, clientaddr)
		if err != nil {
			log.Println("write request finished, with error:")
			log.Println(err)
		}
	default:
		log.Println("Invalid Packet Type!")
	}
}

// Serve opens up a udp socket listening on the given
// address and handles incoming connections received on it
func (s *Server) Serve(addr string) error {
	uaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	uconn, err := net.ListenUDP("udp", uaddr)
	if err != nil {
		return err
	}

	for { // read in new requests
		buf := make([]byte, TftpMaxPacketSize) // TODO: sync.Pool
		n, ua, err := uconn.ReadFromUDP(buf)
		if err != nil {
			return err
		}

		log.Println("New Connection!")

		buf = buf[:n]
		packet, err := pkt.ParsePacket(buf)
		if err != nil {
			log.Printf("Got bad packet: %s", err)
			continue
		}

		go s.HandleClient(ua, packet)
	}
}