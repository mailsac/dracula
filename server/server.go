package server

import (
	"errors"
	"fmt"
	"github.com/mailsac/throttle-counter/protocol"
	"github.com/mailsac/throttle-counter/store"
	"net"
	"strconv"
)

type Server struct {
	store    *store.Store
	conn     *net.UDPConn
	disposed bool
	Debug    bool
}

func NewServer(expireAfterSecs int64) *Server {
	return &Server{store: store.NewStore(expireAfterSecs)}
}
func (s *Server) Listen(udpPort int) error {
	if s.conn != nil {
		return errors.New("server already initialized")
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: udpPort,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		return err
	}
	//defer conn.Close()
	s.conn = conn
	fmt.Printf("server listening %s\n", conn.LocalAddr().String())

	go s.handleForever()

	return nil
}

func (s *Server) Close() error {
	if s.disposed {
		return nil
	}
	s.disposed = true
	s.store.DisableCleanup()
	err := s.conn.Close()
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) handleForever() {
	for {
		if s.disposed {
			break
		}
		message := make([]byte, protocol.PacketSize)
		_, remote, err := s.conn.ReadFromUDP(message[:])
		if err != nil {
			fmt.Println("server read error:", err)
			continue
		}
		packet, err := protocol.ParsePacket(message)
		if err != nil {
			p := protocol.Packet{
				Command:   protocol.ResError,
				MessageID: packet.MessageID,
				Namespace: packet.Namespace,
				DataValue: []byte(err.Error()),
			}
			s.respondOrLogError(remote, &p)
			continue
		}

		if s.Debug {
			fmt.Println("server received packet:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
		}

		switch packet.Command {
		case protocol.CmdPut:
			s.store.Put(packet.NamespaceString(), packet.DataValueString())
			p := protocol.Packet{
				Command:   protocol.CmdPut,
				MessageID: packet.MessageID,
				Namespace: packet.Namespace,
			}
			s.respondOrLogError(remote, &p)
			break
		case protocol.CmdCount:
			count := s.store.Count(packet.NamespaceString(), packet.DataValueString())
			p := protocol.Packet{
				Command:   protocol.CmdCount,
				MessageID: packet.MessageID,
				Namespace: packet.Namespace,
				DataValue: []byte(strconv.Itoa(count)),
			}
			s.respondOrLogError(remote, &p)
			break
		default:
			p := protocol.Packet{
				Command:   protocol.ResError,
				MessageID: packet.MessageID,
				Namespace: packet.Namespace,
				DataValue: []byte("unknown_command_" + string(packet.Command)),
			}
			s.respondOrLogError(remote, &p)
			break
		}
	}
}

func (s *Server) respondOrLogError(addr *net.UDPAddr, packet *protocol.Packet) {
	if s.Debug {
		fmt.Println("server sending packet:", addr, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
	}
	b, err := packet.Bytes()
	if err != nil {
		fmt.Println("server error: constructing packet for response", addr, err, packet)
		return
	}
	_, err = s.conn.WriteToUDP(b, addr)
	if err != nil {
		fmt.Println("server error: responding", addr, err, packet)
		return
	}
}
