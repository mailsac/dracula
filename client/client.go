package client

import (
	"errors"
	"fmt"
	"github.com/mailsac/dracula/client/waitingmessage"
	"github.com/mailsac/dracula/protocol"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrMessageTimedOut = errors.New("timed out waiting for message response")
	ErrClientAlreadyInit = errors.New("client already initialized")
)

type Client struct {
	conn            *net.UDPConn
	remoteServer    *net.UDPAddr
	messagesWaiting *waitingmessage.ResponseCache // byte is the expected response command type

	messageIDCounter uint32

	disposed bool
	Debug    bool
}

func NewClient(remoteServerIP string, remoteUDPPort int, timeout time.Duration) *Client {
	return &Client{
		remoteServer: &net.UDPAddr{
			Port: remoteUDPPort,
			IP:   net.ParseIP(remoteServerIP),
		},
		messagesWaiting: waitingmessage.NewCache(timeout),
	}
}

func (c *Client) PendingRequests() int {
	return c.messagesWaiting.Len()
}

func (c *Client) Listen(localUDPPort int) error {
	if c.conn != nil {
		return ErrClientAlreadyInit
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: localUDPPort,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		return err
	}
	//defer conn.Close()
	c.conn = conn
	if c.Debug {
		fmt.Printf("client listening %s\n", conn.LocalAddr().String())
	}

	go c.handleResponsesForever()
	go c.handleTimeouts()

	return nil
}

func (c *Client) Close() error {
	var err error
	if c.disposed {
		return nil
	}
	c.disposed = true
	c.messagesWaiting.Dispose()

	if c.conn != nil {
		err = c.conn.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) handleTimeouts() {
	for timedOutCallback := range c.messagesWaiting.TimedOutMessages {
		timedOutCallback("", ErrMessageTimedOut)
		if c.disposed {
			break
		}
	}
}

func (c *Client) handleResponsesForever() {
	for {
		if c.disposed {
			break
		}
		message := make([]byte, protocol.PacketSize)
		_, remote, err := c.conn.ReadFromUDP(message[:])
		if err != nil {
			fmt.Println("client read error:", err)
			continue
		}
		packet, err := protocol.ParsePacket(message)
		if err != nil {
			if packet != nil && packet.MessageID > 0 {
				fmt.Println("client parse packet error but has message id:", packet.MessageID, remote, err, message)
			} else {
				fmt.Println("client received invalid packet from", remote, err, message)
				continue
			}
		}

		if c.Debug {
			fmt.Println("client received packet:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
		}

		cb, err := c.messagesWaiting.Pull(packet.MessageID)
		if err != nil {
			fmt.Println("client message not expected:", packet.Command, packet.MessageID, packet.NamespaceString(), err)
			continue
		}

		if !protocol.IsResponseCmd(packet.Command) {
			fmt.Println("client message not response command:", packet.Command, packet.MessageID, packet.NamespaceString())
			continue
		}

		// handle packet error by constructing error from data value
		if packet.Command == protocol.ResError {
			cb("", errors.New(packet.DataValueString()))
			continue
		}

		if packet.Command == protocol.CmdCount || packet.Command == protocol.CmdPut {
			cb(packet.DataValueString(), nil)
			continue
		}

		fmt.Println("client unhandled valid response!", packet.Command, packet.MessageID, packet.NamespaceString())
	}
}

func (c *Client) getMessageID() uint32 {
	atomic.AddUint32(&c.messageIDCounter, 1)

	return c.messageIDCounter
}

// Count creates a callback and returns the result of it
func (c *Client) Count(namespace, value string) (int, error) {
	messageID := c.getMessageID()
	var wg sync.WaitGroup
	var output int
	var err error
	cb := func(o string, e error) {
		if err != nil {
			err = e
		} else {
			i, err := strconv.ParseInt(o, 10, 64)
			if err != nil {
				err = e
			} else {
				output = int(i)
			}
		}
		wg.Done()
	}
	wg.Add(1)
	// callback has been setup, now make the request
	c.sendOrCallbackErr(&protocol.Packet{
		Command:   protocol.CmdCount,
		MessageID: messageID,
		Namespace: []byte(namespace),
		DataValue: []byte(value),
	}, cb)

	wg.Wait() // wait for callback to be called
	return output, err
}

func (c *Client) Put(namespace, value string) error {
	messageID := c.getMessageID()
	var wg sync.WaitGroup
	var err error
	cb := func(_ string, e error) {
		err = e
		wg.Done()
	}
	wg.Add(1)
	// callback has been setup, now make the request
	c.sendOrCallbackErr(&protocol.Packet{
		Command:   protocol.CmdPut,
		MessageID: messageID,
		Namespace: []byte(namespace),
		DataValue: []byte(value),
	}, cb)

	wg.Wait() // wait for callback to be called
	return err
}

func (c *Client) sendOrCallbackErr(packet *protocol.Packet, cb waitingmessage.Callback) {
	if c.Debug {
		fmt.Println("client sending packet:", string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
	}

	b, err := packet.Bytes()
	if err != nil {
		// probably bad packet
		cb("", err)
		return
	}

	err = c.messagesWaiting.Add(packet.MessageID, cb)
	if err != nil {
		fmt.Println("client failed adding waiting message!", packet.MessageID)
		cb("", err)
		return
	}

	_, err = c.conn.WriteToUDP(b, c.remoteServer)
	if err != nil {
		// immediate failure, handle here
		reCall, err := c.messagesWaiting.Pull(packet.MessageID)
		if err != nil {
			fmt.Println("client failed callback could not be called!", packet.MessageID)
			reCall = cb
		}
		reCall("", err)
		return
	}

	// ok
}
