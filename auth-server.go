/*
Implements a UDP server(aserver) which receives client request, authorizes valid client, and sends client address of fserver.

Usage:
$ go run auth-server.go [aserver UDP ip:port] [fserver RPC ip:port] [secret]
[aserver UDP ip:port] : the UDP address on which the aserver receives new client connections
[fserver RPC ip:port] : the TCP address on which the fserver listens to RPC connections from the aserver
[secret] : an int64 secret

*/

package main

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"sync"
)

/////////// Msgs used by both auth and fortune servers:

// An error message from the server.
type ErrMessage struct {
	Error string
}

/////////// Auth server msgs:

// Message containing a nonce from auth-server.
type NonceMessage struct {
	Nonce int64
}

// Message containing an MD5 hash from client to auth-server.
type HashMessage struct {
	Hash string
}

// Message with details for contacting the fortune-server.
type FortuneInfoMessage struct {
	FortuneServer string
	FortuneNonce  int64
}

// Keep track of client state
type ClientRecords struct {
	m   map[string]int64 // a map holding nonce value assigned to each client
	mux sync.Mutex       // a mutex to make sure that map m is always synced
}

// Main workhorse method.
func main() {
	// parse inputs
	aserverAddr := os.Args[1]
	fserverAddr := os.Args[2]
	secretStr := os.Args[3]
	secret, err := strconv.ParseInt(secretStr, 10, 64)
	printErr(err, "Argument Parsing")
	//	fmt.Printf("local: %s, server: %s, secret: %v\n", localAddr, serverAddr, secret)

	// resolve address
	aAddr, err := net.ResolveUDPAddr("udp", aserverAddr)
	printErr(err, "resolve UDP address")

	// set up server
	aConn, err := net.ListenUDP("udp", aAddr)
	printErr(err, " listen UDP connection")
	defer aConn.Close()

	// client records is a map which records each client's assigned nonce
	records := ClientRecords{m: make(map[string]int64)}

	// connect to fserver
	client, err := rpc.Dial("tcp", fserverAddr)
	printErr(err, "dialing fserver")
	defer client.Close()

	// receive request from clients
	for {
		// receive message from client
		msg := make([]byte, 1024)
		n, cAddr, err := aConn.ReadFromUDP(msg)
		if err == nil {
			fmt.Printf("message: %s received from %s\n", msg[0:n], cAddr)
			// concurrently handle requests
			go handleRequest(aConn, msg[0:n], &records, client, secret, cAddr)
		}
	}
	return

}

func handleRequest(conn *net.UDPConn, msg []byte, record *ClientRecords, rpcClient *rpc.Client, secret int64, cAddr *net.UDPAddr) {
	var hash HashMessage
	err := json.Unmarshal(msg[:], &hash)
	// if received message is not hash, return nonce message
	if err != nil {
		var nonce NonceMessage
		rand.Seed(222)
		newNonce := rand.Int63()

		// update map to record client information and new nonce
		record.mux.Lock()
		record.m[cAddr.String()] = newNonce
		record.mux.Unlock()

		// send nonce back to client
		nonce.Nonce = newNonce
		sendmsg, _ := json.Marshal(nonce)
		conn.WriteToUDP(sendmsg, cAddr)
		return

	} else {
		// if received message is hash, check against saved nonce
		record.mux.Lock()
		validNonce, ok := record.m[cAddr.String()]
		record.mux.Unlock()

		// in case no previous nonce available, report unkown remote client address error
		if !ok {
			var unknownClientError ErrMessage
			unknownClientError.Error = "unknown remote client address"
			errmsg, _ := json.Marshal(unknownClientError)
			conn.WriteToUDP(errmsg, cAddr)
			return
		} else {
			// else check hash value
			value := validNonce + secret
			n := binary.PutVarint(msg, value)
			hashmd5 := md5.Sum(msg[:n])
			hashStr := hex.EncodeToString(hashmd5[:])

			if hashStr == hash.Hash {
				// get fortune nonce from fserver
				var fInfoMsg FortuneInfoMessage
				err = rpcClient.Call("FortuneServerRPC.GetFortuneInfo", cAddr.String(), &fInfoMsg)
				if err == nil {
					replymsg, _ := json.Marshal(fInfoMsg)
					conn.WriteToUDP(replymsg, cAddr)
					return
				}

			} else {
				// report invalid hash error
				var invalidHashError ErrMessage
				invalidHashError.Error = "unexpected hash value"
				replymsg, _ := json.Marshal(invalidHashError)
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
