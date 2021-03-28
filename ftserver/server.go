package ftserver

import (
	"crypto/tls"
	"github.com/orestonce/filetransfer/ftclient"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type TransferServer struct {
	ln            net.Listener
	wg            sync.WaitGroup
	lastErr       error
	connMap       map[uint64]*clientTcpTrack
	connMapLocker sync.Mutex
	matchLocker   sync.Mutex
	isClose       chan struct{}
	isCloseLocker sync.Mutex
}

func RunTransferServer(ListenAddr string) (s *TransferServer, err error) {
	CertPem, KeyPem, err := generateCert(generateCertReq{
		hostList:  []string{"self-sign-host"},
		notBefore: time.Now().Add(-time.Hour * 10),
	})
	if err != nil {
		return nil, err
	}
	certObj, err := tls.X509KeyPair(CertPem, KeyPem)
	if err != nil {
		return nil, err
	}
	ln, err := tls.Listen("tcp", ListenAddr, &tls.Config{
		Certificates: []tls.Certificate{certObj},
	})
	if err != nil {
		return nil, err
	}
	s = &TransferServer{
		ln:      ln,
		isClose: make(chan struct{}),
	}
	s.wg.Add(2)
	go s.start()
	go s.gcConnMap()
	return s, nil
}

func (this *TransferServer) start() {
	defer this.wg.Done()
	var connIdAlloc uint64

	for {
		newConn, err := this.ln.Accept()
		if err != nil {
			this.lastErr = err
			break
		}
		connIdAlloc++
		go this.handleNewConn(connIdAlloc, newConn)
	}
	this.isCloseLocker.Lock()
	close(this.isClose)
	this.isCloseLocker.Unlock()
}

func (this *TransferServer) gcConnMap() {
	defer this.wg.Done()

	isClose := false
	for {
		select {
		case <-this.isClose:
			isClose = true
			break
		case <-time.After(time.Second * 10):
			break
		}
		if isClose {
			break
		}
		now := time.Now()
		for _, track := range this.getClientTrackList() {
			mode, lastRead := track.getState()
			if now.Sub(lastRead) > time.Minute && (mode == modeReceiveWaitConnect || mode == modeSendConnected) {
				track.Close()
			}
		}
	}
}

func (this *TransferServer) Close() (err error) {
	_ = this.ln.Close()
	this.wg.Wait()
	for _, track := range this.getClientTrackList() {
		track.Close()
	}
	return this.lastErr
}

func (this *TransferServer) getClientTrackList() (list []*clientTcpTrack) {
	this.connMapLocker.Lock()
	for _, track := range this.connMap {
		list = append(list, track)
	}
	this.connMapLocker.Unlock()
	return list
}

func (this *TransferServer) handleNewConn(connId uint64, conn net.Conn) {
	packet, err := ftclient.ReadPacket(conn)
	if err != nil {
		log.Println("TransferServer ReadPacket error", err)
		_ = conn.Close()
		return
	}

	track := &clientTcpTrack{
		connId:            connId,
		locker:            sync.Mutex{},
		conn:              conn,
		closeFnList:       nil,
		isClosed:          false,
		password:          packet.Password,
		lastReadTime:      time.Now(),
		sendContentLength: packet.SendContentLength,
	}
	switch packet.Cmd {
	case ftclient.CmdSend:
		track.mode = modeSendWaitConnect
		if !this.setTrack(track) {
			log.Println("TransferServer.handleNewConn3 内部错误", connId)
			writeCmdError(conn, "内部错误")
			track.Close()
			return
		}
	case ftclient.CmdReceive:
		track.mode = modeReceiveWaitConnect
		if !this.setTrack(track) {
			log.Println("TransferServer.handleNewConn4 内部错误", connId)
			writeCmdError(conn, "内部错误")
			track.Close()
			return
		}
	default:
		log.Println("TransferServer.handleNewConn4 error ", packet.Cmd)
		_ = conn.Close()
		return
	}
	sender, receiver := this.getSenderAndReceiver(packet.Password)
	this.handleSenderAndReceiver(sender, receiver)
}

func (this *clientTcpTrack) setState(mode string, now time.Time) {
	this.locker.Lock()
	this.mode = mode
	this.lastReadTime = now
	this.locker.Unlock()
}

func (this *clientTcpTrack) getState() (mode string, lastReadTime time.Time) {
	this.locker.Lock()
	mode = this.mode
	lastReadTime = this.lastReadTime
	this.locker.Unlock()
	return mode, lastReadTime
}

func (this *TransferServer) getSenderAndReceiver(password string) (sender *clientTcpTrack, receiver *clientTcpTrack) {
	this.matchLocker.Lock()
	defer this.matchLocker.Unlock()

	this.connMapLocker.Lock()
	for _, track := range this.connMap {
		if track.password != password {
			continue
		}
		if track.mode == modeSendWaitConnect && sender == nil {
			sender = track
			continue
		}
		if track.mode == modeReceiveWaitConnect && receiver == nil {
			receiver = track
			continue
		}
	}
	this.connMapLocker.Unlock()

	if sender != nil && receiver != nil {
		now := time.Now()
		sender.locker.Lock()
		sender.mode = modeSendConnected
		sender.lastReadTime = now
		sender.locker.Unlock()
		receiver.locker.Lock()
		receiver.mode = modeReceiveConnected
		receiver.lastReadTime = now
		receiver.locker.Unlock()
	}
	return sender, receiver
}

func writeCmdError(conn net.Conn, errMsg string) {
	_ = ftclient.WritePacket(conn, ftclient.Packet{
		Cmd:    ftclient.CmdError,
		ErrMsg: errMsg,
	})
}

func writeSimpleCmd(conn net.Conn, cmd string) (err error) {
	return ftclient.WritePacket(conn, ftclient.Packet{
		Cmd: cmd,
	})
}

func (this *TransferServer) getTrackByPasswordNoLock(password string) (track *clientTcpTrack) {
	for _, oldTrack := range this.connMap {
		if oldTrack.password == password {
			return oldTrack
		}
	}
	return nil
}

func (this *TransferServer) setTrack(track *clientTcpTrack) (success bool) {
	this.connMapLocker.Lock()
	if this.connMap == nil {
		this.connMap = map[uint64]*clientTcpTrack{}
	} else {
		for _, existsTrack := range this.connMap {
			if existsTrack.password == track.password && existsTrack.mode == track.mode {
				this.connMapLocker.Unlock()
				return false
			}
		}
	}
	this.connMap[track.connId] = track
	this.connMapLocker.Unlock()

	track.AddOnClose(func() {
		this.connMapLocker.Lock()
		delete(this.connMap, track.connId)
		this.connMapLocker.Unlock()
	})
	return true
}

func (this *TransferServer) handleSenderAndReceiver(sender *clientTcpTrack, receiver *clientTcpTrack) {
	if sender == nil || receiver == nil {
		return
	}
	for _, oneConn := range []net.Conn{sender.conn, receiver.conn} {
		err := ftclient.WritePacket(oneConn, ftclient.Packet{
			Cmd:               ftclient.CmdSuccess,
			SendContentLength: sender.sendContentLength,
		})
		if err != nil {
			log.Println("TransferServer.handleReceiveReq error 写回应失败", err)
			sender.Close()
			receiver.Close()
			return
		}
	}
	_, err := io.CopyN(receiver.conn, sender.conn, int64(sender.sendContentLength))
	if err != nil {
		log.Println("TransferServer.handleReceiveReq error 传送失败", err)
		sender.Close()
		receiver.Close()
		return
	}
	err = writeSimpleCmd(sender.conn, ftclient.CmdBye)
	if err != nil {
		log.Println("TransferServer.handleReceiveReq error 传送失败2", err)
		sender.Close()
		receiver.Close()
		return
	}
	err = writeSimpleCmd(receiver.conn, ftclient.CmdBye)
	if err != nil {
		log.Println("TransferServer.handleReceiveReq error 传送失败3", err)
		sender.Close()
		receiver.Close()
		return
	}
	sender.Close()
	receiver.Close()
}

type clientTcpTrack struct {
	connId            uint64
	locker            sync.Mutex
	conn              net.Conn
	closeFnList       []func()
	isClosed          bool
	mode              string
	lastReadTime      time.Time
	password          string
	sendContentLength uint64
}

const (
	modeSendWaitConnect    = "modeSendWaitConnect"
	modeSendConnected      = "modeSendConnected"
	modeReceiveWaitConnect = "modeReceiveWaitConnect"
	modeReceiveConnected   = "modeReceiveConnected"
)

func (this *clientTcpTrack) AddOnClose(fn func()) {
	this.locker.Lock()
	if this.isClosed {
		this.locker.Unlock()
		fn()
	} else {
		this.closeFnList = append(this.closeFnList, fn)
		this.locker.Unlock()
	}
}

func (this *clientTcpTrack) Close() {
	this.locker.Lock()
	if this.isClosed {
		this.locker.Unlock()
		return
	}
	_ = this.conn.Close()
	this.isClosed = true
	this.locker.Unlock()

	for _, fn := range this.closeFnList {
		fn()
	}
}
