package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"

	"golang.org/x/crypto/ssh"
)

var privateKeyFlag = flag.String("privateKey", "", "")
var hostFlag = flag.String("host", "", "")
var userFlag = flag.String("user", "", "")

func connectSSH() *ssh.Client {
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

	return client
}

type Socks5Server struct {
	Dial func(n, addr string) (net.Conn, error)
}

func (s *Socks5Server) socks5Auth(conn net.Conn) error {
	buf := make([]byte, 256)

	// 读取 VER 和 NMETHODS
	n, err := io.ReadFull(conn, buf[:2])
	if n != 2 {
		return errors.New("reading header: " + err.Error())
	}

	ver, nMethods := int(buf[0]), int(buf[1])
	if ver != 5 {
		return errors.New("invalid version")
	}

	// 读取 METHODS 列表
	n, err = io.ReadFull(conn, buf[:nMethods])
	if n != nMethods {
		return errors.New("reading methods: " + err.Error())
	}

	n, err = conn.Write([]byte{0x05, 0x00})
	if n != 2 || err != nil {
		return errors.New("write rsp err: " + err.Error())
	}

	return nil
}

func (s *Socks5Server) socks5Connect(client net.Conn) (net.Conn, error) {
	buf := make([]byte, 256)

	n, err := io.ReadFull(client, buf[:4])
	if n != 4 {
		return nil, errors.New("read header: " + err.Error())
	}

	ver, cmd, _, atyp := buf[0], buf[1], buf[2], buf[3]
	if ver != 5 || cmd != 1 {
		return nil, errors.New("invalid ver/cmd")
	}

	addr := ""
	switch atyp {
	case 1:
		n, err = io.ReadFull(client, buf[:4])
		if n != 4 {
			return nil, errors.New("invalid IPv4: " + err.Error())
		}
		addr = fmt.Sprintf("%d.%d.%d.%d", buf[0], buf[1], buf[2], buf[3])

	case 3:
		n, err = io.ReadFull(client, buf[:1])
		if n != 1 {
			return nil, errors.New("invalid hostname: " + err.Error())
		}
		addrLen := int(buf[0])

		n, err = io.ReadFull(client, buf[:addrLen])
		if n != addrLen {
			return nil, errors.New("invalid hostname: " + err.Error())
		}
		addr = string(buf[:addrLen])

	case 4:
		return nil, errors.New("IPv6: no supported yet")

	default:
		return nil, errors.New("invalid atyp")
	}

	n, err = io.ReadFull(client, buf[:2])
	if n != 2 {
		return nil, errors.New("read port: " + err.Error())
	}
	port := binary.BigEndian.Uint16(buf[:2])

	destAddrPort := fmt.Sprintf("%s:%d", addr, port)
	dest, err := s.Dial("tcp", destAddrPort)
	if err != nil {
		return nil, errors.New("dial dst: " + err.Error())
	}

	n, err = client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	if err != nil {
		dest.Close()
		return nil, errors.New("write rsp: " + err.Error())
	}
	return dest, nil
}

func (s *Socks5Server) socks5Forward(src, target net.Conn) {
	forward := func(src, dest net.Conn) {
		defer src.Close()
		defer dest.Close()
		io.Copy(src, dest)
	}
	go forward(src, target)
	go forward(target, src)
}

func (s *Socks5Server) socks5Process(conn net.Conn) {
	if err := s.socks5Auth(conn); err != nil {
		fmt.Println("auth error:", err)
		conn.Close()
		return
	}

	target, err := s.socks5Connect(conn)
	if err != nil {
		fmt.Println("connect error:", err)
		conn.Close()
		return
	}

	s.socks5Forward(conn, target)
}

func HTTPOverSSH(client *ssh.Client) {
	httpClient := http.Client{Transport: &http.Transport{Dial: func(network, addr string) (net.Conn, error) {
		log.Printf("Dial %v with %v", addr, network)
		return client.Dial(network, addr)
	}}}

	resp, err := httpClient.Get("https://www.baidu")
	if err != nil {
		log.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Resp: %v", string(b))
}

func main() {
	flag.Parse()

	client := connectSSH()
	defer client.Close()

	ss := Socks5Server{
		Dial: func(n, addr string) (net.Conn, error) {
			return client.Dial(n, addr)
		},
	}

	server, err := net.Listen("tcp", "0.0.0.0:7788")
	if err != nil {
		fmt.Printf("Listen failed: %v\n", err)
		return
	}

	for {
		conn, err := server.Accept()
		if err != nil {
			fmt.Printf("Accept failed: %v", err)
			continue
		}
		go ss.socks5Process(conn)
	}

}
