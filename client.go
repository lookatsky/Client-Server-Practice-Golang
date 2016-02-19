/*
A client which first connects to aserver for authorization, then connect to fserver to grab fortune message.  

Usage:
$ go run client.go [local UDP ip:port] [aserver UDP ip:port] [secret]

Example:
$ go run client.go 127.0.0.1:2020 127.0.0.1:7070 1984
*/

package main

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
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

/////////// Fortune server msgs:

// Message requesting a fortune from the fortune-server.
type FortuneReqMessage struct {
	FortuneNonce int64
}

// Response from the fortune-server containing the fortune.
type FortuneMessage struct {
	Fortune string
}

// Main workhorse method.
func main() {
	// parse inputs
	localAddr := os.Args[1]
	serverAddr := os.Args[2]
	secretStr := os.Args[3]
	secret, err := strconv.ParseInt(secretStr, 10, 64)
	printErr(err)
	//	fmt.Printf("local: %s, server: %s, secret: %v\n", localAddr, serverAddr, secret)
	msg := make([]byte, 1024)

	// sends a UDP message with arbitrary payload to the aserver
	aAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	printErr(err)
	lAddr, err := net.ResolveUDPAddr("udp", localAddr)
	printErr(err)
	aConn, err := net.DialUDP("udp", lAddr, aAddr)
	printErr(err)

	msg[2] = byte(2)
	_, err = aConn.Write(msg)
	printErr(err)

	// receives a NonceMessage reply containing an int64 nonce from the aserver
	n, err := aConn.Read(msg)
	printErr(err)
	//	fmt.Printf("%s\n", msg[0:n])

	var nonce NonceMessage
	err = json.Unmarshal(msg[0:n], &nonce)
	printErr(err)

	// computes an MD5 hash of the (nonce + secret) value and sents this value as a hex string to the aserver as part of a HashMessage
	value := nonce.Nonce + secret
	n = binary.PutVarint(msg, value)
	hash := md5.Sum(msg[:n])
	hashStr := hex.EncodeToString(hash[:])
	//	fmt.Printf("nonce(%v) + secret(%v) : %v; \nMD5 hash: %v\n", nonce.Nonce, secret, value, hashStr)

	var hashMsg HashMessage
	hashMsg.Hash = hashStr
	sendmsg, err := json.Marshal(hashMsg)
	printErr(err)
	_, err = aConn.Write(sendmsg)
	printErr(err)

	// the aserver verifies the received hash and replies with a FortuneInfoMessage
	n, err = aConn.Read(msg)
	printErr(err)
	//	fmt.Printf("%s",msg[0:n])
	var fortuneInfo FortuneInfoMessage
	err = json.Unmarshal(msg[0:n], &fortuneInfo)
	printErr(err)

	// the client sends a FortuneReqMessage to fserver
	fAddr, err := net.ResolveUDPAddr("udp", fortuneInfo.FortuneServer)
	printErr(err)
	err = aConn.Close()
	printErr(err)
	fConn, err := net.DialUDP("udp", lAddr, fAddr)
	printErr(err)

	var fortuneReqMsg FortuneReqMessage
	fortuneReqMsg.FortuneNonce = fortuneInfo.FortuneNonce
	reqMsg, err := json.Marshal(fortuneReqMsg)
	printErr(err)
	_, err = fConn.Write(reqMsg)
	printErr(err)

	// the client receives a fortunemessage from the fserver
	n, err = fConn.Read(msg)
	printErr(err)
	var fMsg FortuneMessage
	err = json.Unmarshal(msg[0:n], &fMsg)
	printErr(err)

	fmt.Println(fMsg.Fortune)

}

func printErr(e error) {
	if e != nil {
		fmt.Println("Error message:", e)
		os.Exit(1)
	}
}