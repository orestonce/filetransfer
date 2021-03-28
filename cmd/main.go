package main

import (
	"crypto/md5"
	"encoding/hex"
	"github.com/orestonce/filetransfer/ftclient"
	"github.com/orestonce/filetransfer/ftserver"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var root = &cobra.Command{
	Use: "ftcmd",
}

var serve = &cobra.Command{
	Use: "serve",
	Run: func(cmd *cobra.Command, args []string) {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		server, err := ftserver.RunTransferServer(serverAddr)
		if err != nil {
			panic(err)
		}
		ch := make(chan os.Signal)
		signal.Notify(ch, syscall.SIGABRT, syscall.SIGHUP, syscall.SIGKILL)
		<-ch
		_ = server.Close()
	},
}

var serverAddr string
var password string
var sendFileName string

var send = &cobra.Command{
	Use: "send",
	Run: func(cmd *cobra.Command, args []string) {
		data, err := ioutil.ReadFile(sendFileName)
		if err != nil {
			log.Println("ioutil.ReadFile error", sendFileName, err)
			return
		}
		err0 := ftclient.SendData(serverAddr, password, data)
		log.Println("send", err0, md5Hex(data))
	},
}

func md5Hex(data []byte) string {
	tmp := md5.Sum(data)
	return hex.EncodeToString(tmp[:])
}

var receiveFileName string
var receive = &cobra.Command{
	Use: "receive",
	Run: func(cmd *cobra.Command, args []string) {
		data, err := ftclient.ReceiveData(serverAddr, password)
		if err != nil {
			log.Println("receive", err)
			return
		}
		err = ioutil.WriteFile(receiveFileName, data, 0777)
		log.Println("receive", err, md5Hex(data))
	},
}

func init() {
	serve.Flags().StringVar(&serverAddr, "serverAddr", "127.0.0.1:1234", "本地监听地址")
	root.AddCommand(serve)
	send.Flags().StringVar(&serverAddr, "serverAddr", "127.0.0.1:1234", "服务端地址")
	send.Flags().StringVar(&password, "password", "", "密码")
	send.Flags().StringVar(&sendFileName, "file", "", "要发送的文件")
	root.AddCommand(send)
	receive.Flags().StringVar(&serverAddr, "serverAddr", "127.0.0.1:1234", "服务端地址")
	receive.Flags().StringVar(&password, "password", "", "密码")
	receive.Flags().StringVar(&receiveFileName, "file", "", "接受到的文件保存地址")
	root.AddCommand(receive)
}

func main() {
	if err := root.Execute(); err != nil {
		_ = root.Help()
	}
}
