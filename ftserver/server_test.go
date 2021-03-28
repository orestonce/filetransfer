package ftserver

import (
	"bytes"
	"github.com/orestonce/filetransfer/ftclient"
	"sync"
	"testing"
)

func TestRunTransferServer(t *testing.T) {
	const serverAddr = "127.0.0.1:1234"
	const password = "1234password"
	var sendData = []byte{1, 3, 5, 7, 8, 5, 2, 3, 4, 3, 234, 232, 3, 21, 67, 23}

	server, err := RunTransferServer(serverAddr)
	if err != nil {
		panic(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err0 := ftclient.SendData(serverAddr, password, sendData)
		if err0 != nil {
			panic(err)
		}
	}()
	go func() {
		defer wg.Done()
		data, err1 := ftclient.ReceiveData(serverAddr, password)
		if err1 != nil {
			panic(err)
		}
		if !bytes.Equal(data, sendData) {
			panic("receive data not equal.")
		}
	}()
	wg.Wait()
	_ = server.Close()
}

