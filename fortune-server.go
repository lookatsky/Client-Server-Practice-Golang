/*
Implemented a fortune server which receives client request via UDP, and replies with a fortune message if client is authorized.
It checks client authorization by connecting to an authorization server (aserver) via RPC.

Usage:
$ go run fortune-server.go [fserver RPC ip:port] [fserver UDP ip:port] [fortune-string]
[fserver RPC ip:port] : the TCP address on which the fserver listens to RPC connections from the aserver
[fserver UDP ip:port] : the UDP address on which the fserver receives client connections
[fortune-string] : a fortune string that may include spaces, but not other whitespace characters
*/

package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"sync"
)

/////////// Msgs used by both auth and fortune servers:

// An error message from the server.
type ErrMessage struct {
	Error string
}

/////////// Fortune server msgs:

// Message requesting a fortune from the fortune-server.
type FortuneReqMessage struct {
	FortuneNonce int64
}

// Response from the fortune-server containing the fortune.
type FortuneMessage struct {
	Fortune string
}

// Message with details for contacting the fortune-server.
type FortuneInfoMessage struct {
	FortuneServer string
	FortuneNonce  int64
}

type FortuneServerRPC struct {
	m      map[string]int64
	mux    sync.Mutex
	server string
}

func (this *FortuneServerRPC) GetFortuneInfo(clientAddr string, fInfoMsg *FortuneInfoMessage) error {
	rand.Seed(110)
	newNonce := rand.Int63()
	this.mux.Lock()
	this.m[clientAddr] = newNonce
	this.mux.Unlock()

	fInfoMsg.FortuneNonce = newNonce
	fInfoMsg.FortuneServer = this.server

	return nil
}

// Main workhorse method.
func main() {
	// parse inputs
	fserverRPC := os.Args[1]
	fserverUDP := os.Args[2]
	fortuneString := os.Args[3]
	//	fmt.Printf("local: %s, server: %s, secret: %v\n", localAddr, serverAddr, secret)

	// receive udp connection from client

	// resolve address
	fAddr, err := net.ResolveUDPAddr("udp", fserverUDP)
	printErr(err, "resolve UDP address")

	// set up server
	fConn, err := net.ListenUDP("udp", fAddr)
	printErr(err, " listen UDP connection")
	defer fConn.Close()

	// serve rpc connection over tcp
	fortuneServerRPC := new(FortuneServerRPC)
	fortuneServerRPC.server = fserverUDP
	fortuneServerRPC.m = make(map[string]int64)
	rpc.Register(fortuneServerRPC)
	l, err := net.Listen("tcp", fserverRPC)
	printErr(err, "listen tcp connection")

	go rpc.Accept(l)

	// handle request from client to udp server
	for {
		// read fortune message from clients
		msg := make([]byte, 1024)
		n, cAddr, err := fConn.ReadFromUDP(msg)
		if err == nil {
			fmt.Printf("message: %s received from %s\n", msg[0:n], cAddr)
			//concurrently handle requests
			go fortune(fConn, msg[0:n], fortuneServerRPC, fortuneString, cAddr)
		}
	}
	return
}

// process individual client request
func fortune(conn *net.UDPConn, msg []byte, fortuneServerRPC *FortuneServerRPC, fortuneString string, cAddr *net.UDPAddr) {
	var fortuneReq FortuneReqMessage
	err := json.Unmarshal(msg[:], &fortuneReq)

	if err != nil {
		// client sent malformed message, reply error
		var malformedMsgError ErrMessage
		malformedMsgError.Error = "could not interpret message"
		malformmsg, _ := json.Marshal(malformedMsgError)
		conn.WriteToUDP(malformmsg, cAddr)
		return
	} else {
		// message valid, check for validity of nonce
		fortuneServerRPC.mux.Lock()
		validNonce, ok := fortuneServerRPC.m[cAddr.String()]
		fortuneServerRPC.mux.Unlock()

		if !ok {
			// client sends a fortune nonce from a different address than it used in communicating with the aserver.
			var unknownClientError ErrMessage
			unknownClientError.Error = "unknown remote client address"
			unknowmmsg, _ := json.Marshal(unknownClientError)
			conn.WriteToUDP(unknowmmsg, cAddr)
			return
		} else {
			if fortuneReq.FortuneNonce != validNonce {
				// client sends incorrect nonce
				var invalidNonceError ErrMessage
				invalidNonceError.Error = "incorrect fortune nonce"
				invalidNoncemsg, _ := json.Marshal(invalidNonceError)
				conn.WriteToUDP(invalidNoncemsg, cAddr)
				return
			} else {
				// client sends correct nonce, reply with fortune message
				var fortuneMsg FortuneMessage
				fortuneMsg.Fortune = fortuneString
				replymsg, _ := json.Marshal(fortuneMsg)
				conn.WriteToUDP(replymsg, cAddr)
				return
			}
		}
	}
}

func printErr(e error, s string) {
	if e != nil {
		fmt.Println("Error on ", s, " : ", e)
		os.Exit(-1)
	}
}
