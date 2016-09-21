package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
)

var src_proto = "tcp"
var src_ipport = "0.0.0.0:30303"
var dest_proto = "tcp"
var dest_ipport = "127.0.0.1:3000"
var dbg = false

func help() {
	fmt.Println("fwd google.com:80 \n\t tcp:0.0.0.0:30303→tcp:google:80")
	fmt.Println("fwd 127.0.0.1:80 google.com:80 \n\t tcp:127.0.0.1:80→tcp:google:80")
	fmt.Println("fwd udp 127.0.0.1:80 google.com:80 \n\t udp:127.0.0.1:80→udp:google:80")
	fmt.Println("fwd udp 127.0.0.1:80 unix /tmp/pipe.sock \n\t udp:127.0.0.1:80→/tmp/pipe.sock")
}

func main() {
	// Config
	arg := os.Args[1:]
	switch len(arg) {
	case 0:
		help()
		return
	case 1:
		dest_ipport = arg[0]
	case 2:
		src_ipport = arg[0]
		dest_ipport = arg[1]
	case 3:
		src_proto = arg[0]
		dest_proto = arg[0]
		src_ipport = arg[1]
		dest_ipport = arg[2]
	case 4:
		src_proto = arg[0]
		src_ipport = arg[1]
		dest_proto = arg[2]
		dest_ipport = arg[3]
	default:
		fmt.Println("Too many args.")
		arg = append(arg, "help")
	}

	// Blocking Server
	fmt.Printf("Waiting for connections on %v %v → %v %v\n",
		src_proto, src_ipport,
		dest_proto, dest_ipport)
	session, _ := net.Listen(src_proto, src_ipport)
	for {
		conn, _ := session.Accept()
		go handleComms(conn)
	}
}

func handleComms(conn net.Conn) {
	rid := grid()
	print("New connection:" + rid + "\n")
	target, err := net.DialTimeout(dest_proto, dest_ipport, 500000)
	if err != nil {
		if dbg {
			print(rid + ":" + err.Error() + "\n")
		}
		conn.Write([]byte(fmt.Sprintf("5XX:%s", err.Error())))
		conn.Close()
		return
	}
	Pipe(conn, target, rid)
}

// Pipe creates a full-duplex pipe between the two sockets and transfers data from one to the other.
func Pipe(conn1 net.Conn, conn2 net.Conn, id string) {
	chan1 := chanFromConn(conn1)
	chan2 := chanFromConn(conn2)
	close := func() {
		conn1.Close()
		conn2.Close()
	}
	for {
		select {
		case b1 := <-chan1:
			if dbg {
				fmt.Printf(id+":Client: %s [eof?%v]\n", b1, b1 == nil)
			}
			if b1 == nil {
				close()
				return
			} else {
				conn2.Write(b1)
			}
		case b2 := <-chan2:
			if dbg {
				fmt.Printf(id+":Server: %s [eof?%v]\n", b2, b2 == nil)
			}
			if b2 == nil {
				close()
				return
			} else {
				conn1.Write(b2)
			}
		}
	}
}

func grid() string {
	out, err := exec.Command("uuidgen").Output()
	if err != nil {
		log.Fatal(err)
	}
	return string(out[:len(out)-1])
}

// chanFromConn creates a channel from a Conn object, and sends everything it
//  Read()s from the socket to the channel.
func chanFromConn(conn net.Conn) chan []byte {
	c := make(chan []byte)

	go func() {
		b := make([]byte, 1024)

		for {
			n, err := conn.Read(b)
			if n > 0 {
				res := make([]byte, n)
				// Copy the buffer so it doesn't get changed while read by the recipient.
				copy(res, b[:n])
				c <- res
			}
			if err != nil {
				c <- nil
				break
			}
		}
	}()

	return c
}