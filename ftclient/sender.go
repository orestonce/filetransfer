package ftclient

import (
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
)

func SendData(saddr string, password string, data []byte) (err error) {
	conn, err := tls.Dial("tcp", saddr, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Println("dial error", err)
		return err
	}
	err = WritePacket(conn, Packet{
		Cmd:               CmdSend,
		Password:          password,
		SendContentLength: uint64(len(data)),
	})
	if err != nil {
		log.Println("write send cmd error", err)
		_ = conn.Close()
		return err
	}
	_, err = clientReadAndHandleError(conn, CmdSuccess)
	if err != nil {
		_ = conn.Close()
		return err
	}
	_, err = conn.Write(data)
	if err != nil {
		log.Println("write error", err)
		_ = conn.Close()
		return err
	}
	_, err = clientReadAndHandleError(conn, CmdBye)
	if err != nil {
		_ = conn.Close()
		return err
	}
	_ = conn.Close()
	return nil
}

func ReceiveData(saddr string, password string) (data []byte, err error) {
	conn, err := tls.Dial("tcp", saddr, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, err
	}
	err = WritePacket(conn, Packet{
		Cmd:      CmdReceive,
		Password: password,
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	reply, err := clientReadAndHandleError(conn, CmdSuccess)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	data = make([]byte, reply.SendContentLength)
	_, err = io.ReadFull(conn, data)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_, err = clientReadAndHandleError(conn, CmdBye)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func clientReadAndHandleError(conn net.Conn, expectCmd string) (p Packet, err error) {
	reply, err := ReadPacket(conn)
	if err != nil {
		log.Println("clientReadAndHandleError read cmd error", err)
		return p, err
	}
	switch reply.Cmd {
	case expectCmd:
		break
	case CmdError:
		log.Println("clientReadAndHandleError reply cmd error", reply.ErrMsg)
		return p, err
	default:
		log.Println("clientReadAndHandleError reply error", reply)
		return p, errors.New("reply error")
	}
	return reply, nil
}
