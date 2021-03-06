package server

import (
	"io"
	"log"
	"net"
	"time"

	pkt "github.com/muazrusli/ind-pro/packet"
)

// HandleReadReq handles a new read request with a client, sending them
// the requested file if it exists.
func (s *Server) HandleReadReq(rrq *pkt.ReqPacket, addr *net.UDPAddr) error {
	log.Printf("Read Request: %s", rrq.Filename)
	log.Printf("Dialing out %s", addr.String())

	// 'Our' Address
	listaddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return err
	}

	// Connection directly to their open port
	con, err := net.DialUDP("udp", listaddr, addr)
	if err != nil {
		return err
	}

	// Open whatever file it is that the client desires,
	// no questions asked (TODO: enforce root locking)
	fi, err := s.ReadFunc(s.servdir + "/" + rrq.Filename)
	if err != nil {
		return err
	}

	buf := make([]byte, 512)
	blknum := uint16(1)
	var done bool
	for len(buf) == 512 && !done {
		n, err := fi.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if err == io.EOF {
			done = true
		}

		buf = buf[:n]

		data := &pkt.DataPacket{
			Data:     buf,
			BlockNum: blknum,
		}

		err = sendDataPacket(data, con)
		if err != nil {
			return err
		}
		blknum++
	}
	log.Println("done with transfer")
	return nil
}

// sendDataPacket sends the given data packet to the connected client
// and waits for the correct ACK, or times out
func sendDataPacket(d *pkt.DataPacket, con *net.UDPConn) error {
	//log.Println("Sending data!")
	_, err := con.Write(d.Bytes())
	if err != nil {
		return err
	}

	// Now wait for the ACK...
	maxtimeout := time.After(AckTimeout)
	ackch := make(chan error)

	// Move it to its own function
	go func() {
		ack := make([]byte, 256)
		for {
			n, _, err := con.ReadFromUDP(ack)
			if err != nil {
				ackch <- err
				return
			}

			pack, err := pkt.ParsePacket(ack[:n])
			if err != nil {
				ackch <- err
				return
			}

			// Check packet type
			ackpack, ok := pack.(*pkt.AckPacket)
			if !ok {
				ackch <- pkt.ErrPacketType
				return
			}

			if ackpack.GetBlocknum() != d.BlockNum {
				log.Printf("got ack(%d) but expected ack(%d)\n", d.BlockNum, ackpack.GetBlocknum())
				continue
			}
			ackch <- nil
			return
		}
	}()

	// Loop and retransmit until ack or timeout
	retransmit := time.After(RetransmitTime)
	for {
		select {
		case <-maxtimeout:
			return ErrTimeout
		case <-retransmit:
			log.Println("Retransmit")
			_, err := con.Write(d.Bytes())
			if err != nil {
				return err
			}
			retransmit = time.After(RetransmitTime)
		case err := <-ackch:
			return err
		}
	}
}