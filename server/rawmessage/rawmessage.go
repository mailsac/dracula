package rawmessage

import (
	"bufio"
	"bytes"
	"github.com/mailsac/dracula/protocol"
	"io"
	"log"
	"net"
	"unicode"
)

type RawMessage struct {
	Message        []byte
	Remote         *net.UDPAddr
	MaybeTcpClient *net.TCPConn
}

// ReadOneTcpMessage can be used for the client or server
func ReadOneTcpMessage(l *log.Logger, sendToChannel chan *RawMessage, conn *net.TCPConn) error {
	reader := bufio.NewReader(conn)
	// read lines until full Message is buffered - buffer lives only in this loop
	message, err := reader.ReadBytes('\n')
	if err != nil {
		if err != io.EOF {
			l.Println("ReadOneTcpMessage TCP ReadBytes error:", err, conn.RemoteAddr())
		}
		return err
	}

	// remove spaces and line breaks from the front
	message = bytes.TrimLeftFunc(message, unicode.IsSpace)

	// Check if the Message ends with stop symbol i.e., it's a complete Message.
	// If not, keep reading until we find a complete Message.
	for !bytes.HasSuffix(message, protocol.StopSymbol) {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			l.Println("ReadOneTcpMessage TCP ReadBytes to fill buf error:", err)
			break
		}
		message = append(message, line...)
	}

	// now remove stop symbol
	message = message[0 : len(message)-len(protocol.StopSymbol)]
	message = bytes.TrimRightFunc(message, unicode.IsSpace)
	message = *protocol.PadRight(&message, protocol.PacketSize)

	tcpAddr := conn.RemoteAddr().(*net.TCPAddr)
	sendToChannel <- &RawMessage{
		Message:        message,
		Remote:         &net.UDPAddr{IP: tcpAddr.IP, Port: tcpAddr.Port},
		MaybeTcpClient: conn,
	}
	return nil
}
