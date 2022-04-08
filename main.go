package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
	"net"

	"github.com/armon/go-socks5"
	"golang.org/x/crypto/ssh"
)

var privateKeyFlag = flag.String("privateKey", "", "")
var hostFlag = flag.String("host", "", "")
var userFlag = flag.String("user", "", "")

func main() {
	flag.Parse()

	privateBytes, err := ioutil.ReadFile(*privateKeyFlag)
	if err != nil {
		log.Fatalf("Failed to read private key %v: %v", *privateKeyFlag, err)
	}

	signer, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatal("Failed to parse private key: ", err)
	}

	config := &ssh.ClientConfig{
		User: *userFlag,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", *hostFlag, config)
	if err != nil {
		log.Fatal("Failed to dial: ", err)
	}
	defer client.Close()

	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			log.Printf("Connectiont to %v with %v", addr, network)
			conn, err := client.Dial(network, addr)
			if err != nil {
				log.Printf("Failed to to SSH Dial: %v", err)
			}
			return conn, err
		},
	}
	server, err := socks5.New(conf)
	if err != nil {
		log.Fatal("Failed to create SOCKS5 server: ", err)
	}

	if err := server.ListenAndServe("tcp", "0.0.0.0:7788"); err != nil {
		panic(err)
	}
}
