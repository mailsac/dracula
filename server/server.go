package server

import (
	"errors"
	"fmt"
	"github.com/mailsac/dracula/protocol"
	"github.com/mailsac/dracula/store"
	"math"
	"net"
)

const MinimumExpirySecs = 2

var (
	// ErrExpiryTooSmall means the server was attempted to be initialized with less than MinimumExpirySecs.
	// Values smaller than this are unreliable so they are not allowed.
	ErrExpiryTooSmall    = errors.New("dracula server expiry is too short")
	ErrServerAlreadyInit = errors.New("dracula server already initialized")
)

type Server struct {
	store        *store.Store
	conn         *net.UDPConn
	disposed     bool
	preSharedKey []byte
	Debug        bool
}

func NewServer(expireAfterSecs int64, preSharedKey string) *Server {
	if expireAfterSecs < MinimumExpirySecs {
		panic(ErrExpiryTooSmall)
	}
	psk := []byte(preSharedKey)
	return &Server{store: store.NewStore(expireAfterSecs), preSharedKey: psk}
}

func (s *Server) Listen(udpPort int) error {
	if s.conn != nil {
		return ErrServerAlreadyInit
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
	if s.Debug {
		fmt.Printf("server listening %s\n", conn.LocalAddr().String())
	}

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
			if s.Debug {
				fmt.Println("server received BAD packet:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
			}
			resPacket := protocol.NewPacketFromParts(protocol.ResError, packet.MessageIDBytes, packet.Namespace, []byte(err.Error()), s.preSharedKey)
			s.respondOrLogError(remote, resPacket)
			continue
		}
		err = packet.Validate(s.preSharedKey)
		if err != nil {
			if s.Debug {
				fmt.Println("server got bad hash:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
			}
			resPacket := protocol.NewPacketFromParts(protocol.ResError, packet.MessageIDBytes, packet.Namespace, []byte(err.Error()), s.preSharedKey)
			s.respondOrLogError(remote, resPacket)
			continue
		}

		if s.Debug {
			fmt.Println("server received packet:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
		}

		switch packet.Command {
		case protocol.CmdPut:
			s.store.Put(packet.NamespaceString(), packet.DataValueString())
			resPacket := protocol.NewPacket(protocol.CmdPut, packet.MessageID, packet.NamespaceString(), "", "")
			s.respondOrLogError(remote, resPacket)
			break
		case protocol.CmdCount:
			countInt := s.store.Count(packet.NamespaceString(), packet.DataValueString())
			if countInt > math.MaxUint32 {
				countInt = math.MaxUint32 // prevent overflow
			}
			c := uint32(countInt)
			resPacket := protocol.NewPacketFromParts(protocol.CmdCount, packet.MessageIDBytes, packet.Namespace, protocol.Uint32ToBytes(c), s.preSharedKey)
			s.respondOrLogError(remote, resPacket)
			break
		case protocol.CmdCountNamespace:
			countInt := s.store.CountEntries(packet.NamespaceString())
			if countInt > math.MaxUint32 {
				countInt = math.MaxUint32 // prevent overflow
			}
			c := uint32(countInt)
			resPacket := protocol.NewPacketFromParts(protocol.CmdCountNamespace, packet.MessageIDBytes, packet.Namespace, protocol.Uint32ToBytes(c), s.preSharedKey)
			s.respondOrLogError(remote, resPacket)
			break
		case protocol.CmdCountServer:
			countInt := s.store.CountServerEntries()
			if countInt > math.MaxUint32 {
				countInt = math.MaxUint32 // prevent overflow
			}
			c := uint32(countInt)
			resPacket := protocol.NewPacketFromParts(protocol.CmdCountServer, packet.MessageIDBytes, packet.Namespace, protocol.Uint32ToBytes(c), s.preSharedKey)
			s.respondOrLogError(remote, resPacket)
			break
		default:
			resPacket := protocol.NewPacketFromParts(protocol.ResError, packet.MessageIDBytes, packet.Namespace, []byte("unknown_command_"+string(packet.Command)), s.preSharedKey)
			s.respondOrLogError(remote, resPacket)
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
