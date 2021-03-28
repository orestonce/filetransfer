package ftclient

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

const (
	CmdSend    = "send"
	CmdReceive = "receive"
	CmdError   = "error"
	CmdSuccess = "success"
	CmdBye     = "bye"
)

type Packet struct {
	Cmd               string `json:",omitempty"`
	Password          string `json:",omitempty"` // CmdSend,CmdReceive
	SendContentLength uint64 `json:",omitempty"` // CmdSend
	ErrMsg            string `json:",omitempty"` // CmdError
}

const maxPacketSize = 1024 * 1024 * 1

func ReadPacket(conn net.Conn) (p Packet, err error) {
	err = conn.SetReadDeadline(time.Now().Add(time.Second * 10))
	if err != nil {
		return p, err
	}
	var tmp = make([]byte, 4)

	_, err = io.ReadFull(conn, tmp)
	if err != nil {
		return p, err
	}
	sz := int(binary.BigEndian.Uint32(tmp))
	if sz <= 0 || sz > maxPacketSize {
		return p, errors.New("packet too large " + strconv.Itoa(sz))
	}
	tmp = make([]byte, sz)
	_, err = io.ReadFull(conn, tmp)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(tmp, &p)
	if err != nil {
		return p, err
	}
	err = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return p, err
	}
	return p, nil
}

func WritePacket(conn net.Conn, p Packet) (err error) {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > maxPacketSize {
		return errors.New("packet too large " + strconv.Itoa(len(data)))
	}
	var header = make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	data = append(header, data...)
	err = conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	if err != nil {
		return err
	}
	_, err = conn.Write(data)
	if err != nil {
		return err
	}
	err = conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return err
	}
	return nil
}
