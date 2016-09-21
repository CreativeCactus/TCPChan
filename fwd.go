package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

//Print warnings to stdout, unless we are piping to stdout
var dbg = 1

func main() {
	// Set up the configuration
	src_flag := "tcp:127.0.0.1:81"
	dst_flag := "tcp:127.0.0.1:80"
	fnc_flag := "none"
	flag.StringVar(&src_flag, "src", src_flag, "Where we will listen for connections, eg. udp:127.0.0.1:22")
	flag.StringVar(&dst_flag, "dst", dst_flag, "Where we will forward data to, eg. unix:/tmp/pipe.sock")
	flag.StringVar(&fnc_flag, "fnc", fnc_flag, "Optional encoding behaviour.")
	dbgptr := flag.Int("v", -1, "Level of verbosity. -1=default, 0=none, 3=debug")
	helpopt := flag.Bool("?", false, "Show help")
	flag.Parse()
	dbg = *dbgptr

	help := *helpopt || *flag.Bool("help", false, "Show help") || *flag.Bool("h", false, "Show help")
	if help {
		fmt.Print(`TCPChan fwd v20160921
Usage: fwd [OPTIONS] -src=[src] -dst=[dst]
		-src 	the proto:path:port to listen on
		-dst 	the proto:path:port to forward to
		-fnc	beta. Define an encoding/decoding behaviour
		-v   	verbosity. default lvl 1. If proto out is std, default 0.
		
		proto	one of file/std/unix/udp/tcp
				file and unix do not require a port
				std does not require a path or port
				* if a file has a port value it will be used as the buffer size, default 128, 0=newline delimited 
		path	in the case of file or unix, a file or socket respectively
				in the case of upd or tcp, an IP address or hostname
		port	the port of the IP or hostname
		
Examples
		fwd -src=tcp:127.0.0.1:3000 -dst=file:/tmp/output
		fwd -src=unix:/tmp/in.sock  -dst=udp:10.0.0.1:999
		fwd -src=stdin -dst=stdout
Note: options are not heavily checked
Use fwd --help for more usage
`)
		return
	}

	// Parse parameters
	split := [3]string{}
	copy(split[:], strings.SplitN(src_flag, ":", 3))
	srcProto := split[0]
	srcIP := split[1]
	srcPort := split[2]
	split = [3]string{}
	copy(split[:], strings.SplitN(dst_flag, ":", 3))
	dstProto := split[0]
	dstIP := split[1]
	dstPort := split[2]
	split = [3]string{}
	copy(split[:], strings.SplitN(fnc_flag, ":", 3))
	fncMode := split[0]
	fncKey := split[1]

	if dbg == -1 {
		if dstProto == "std" {
			dbg = 0
		} else {
			dbg = 1
		}
	}

	fmt.Printf("%v %v:%v → %v %v:%v \t[verbosity:%v]\n",
		srcProto, srcIP, srcPort, dstProto, dstIP, dstPort, dbg)

	// Create streams
	input := InputChannel(srcProto, srcIP, srcPort)
	output := OutputChannel(dstProto, dstIP, dstPort)
	encoder := ProxyIO{mode: fncMode, key: fncKey}

	//serve intput to output
	for io := range input {
		enc := encoder
		enc.source = io
		output <- &enc
		defer io.Close()
	}
}

/**
	//ENCODE DEFINITIONS
**/
type ProxyIO struct {
	source AnyIn
	mode   string
	key    string
}

func (p *ProxyIO) Init()                         { p.source.Init() }
func (p *ProxyIO) Read(buf *[]byte) (int, error) { return p.source.Read(buf) }
func (p *ProxyIO) Write(data []byte)             { p.source.Write(data) }
func (p *ProxyIO) Close()                        { p.source.Close() }
func (p *ProxyIO) IsOpen() bool                  { return p.source.IsOpen() }
func (p *ProxyIO) Name() string {
	if p.mode != "none" {
		return "{" + p.mode + "}(" + p.source.Name() + ")"
	}
	return p.source.Name()
}

/**
	//INPUT DEFINITIONS
**/
func InputChannel(proto, path, port string) chan AnyIn {
	inputs := make(chan AnyIn)
	switch proto {
	case "file":
		bufsize := 0
		if bufsize, err := strconv.Atoi(port); err != nil || bufsize == 0 {
			bufsize = 128
		}
		inputs <- &FileIn{path: path, bufsize: bufsize}
	case "std":
		//bufsize := 0
		// if bufsize, err := strconv.Atoi(port); err != nil || bufsize == 0 {
		// 	bufsize = 128
		// }
		inputs <- &StdIn{} //bufsize: bufsize}
	case "tcp", "udp", "unix":
		session, err := net.Listen(proto, path+":"+port)
		if err != nil {
			fmt.Printf("TcpUdpListenErr:%v\n", err.Error())
			panic(err)
		}
		go func() {
			for {
				conn, e := session.Accept()
				if e != nil {
					fmt.Printf("TcpUdpAcceptErr:%v\n", e.Error())
					return
				}
				server := &TCPUDPIn{connection: &conn}
				server.Init()
				print("Debug: sending new tcpudp sess to inputs chan")
				inputs <- server
				//handleComms(conn, dstProto, dstIpport)
			}
		}()
	}
	return inputs
}

type AnyIn interface {
	Init()                     //Initial setup
	Read(*[]byte) (int, error) //Optionally blocking call to get next chunk
	Write([]byte)              //Optional write-back
	Close()                    //Uninit
	IsOpen() bool              //Check if running
	Name() string
}

//STDIN
type StdIn struct {
	input *os.File
	open  bool
}

//Allow 0 for newline delimited?
func (i *StdIn) Init()                         { i.input = os.Stdin; i.open = true }
func (i *StdIn) Read(buf *[]byte) (int, error) { return i.input.Read(*buf) }
func (i *StdIn) Write([]byte)                  { /*By default, do not allow writing back to stdin*/ }
func (i *StdIn) Close()                        { i.open = false }
func (i *StdIn) IsOpen() bool                  { return i.open }
func (i *StdIn) Name() string                  { return "FileIn" }

//FILE
type FileIn struct {
	path    string
	file    *os.File
	buffer  []byte
	bufsize int
	open    bool
}

func (i *FileIn) Init() {
	if file, err := os.Open(i.path); err != nil {
		fmt.Printf("FileInitErr: %v\n", err)
	} else {
		i.file = file
	}
	i.buffer = make([]byte, i.bufsize)
	i.open = true
}
func (i *FileIn) Read(buf *[]byte) (int, error) { return i.file.Read(*buf) }
func (i *FileIn) Write([]byte)                  { /*By default, do not allow writing back to file*/ }
func (i *FileIn) Close()                        { i.file.Close(); i.open = false }
func (i *FileIn) IsOpen() bool                  { return i.open }
func (i *FileIn) Name() string                  { return "FileIn" }

//TCPUDP
type TCPUDPIn struct {
	connection *net.Conn
	isOpen     bool
}

func (i *TCPUDPIn) Init() { i.isOpen = true }
func (i *TCPUDPIn) Read(buf *[]byte) (int, error) {
	buffer := make([]byte, 256)
	n, e := (*i.connection).Read(buffer)
	if n > 0 {
		print("\nDebug: got:" + string(buffer))
	}
	*buf = buffer
	return n, e
}
func (i *TCPUDPIn) Write(data []byte) { (*i.connection).Write(data) }
func (i *TCPUDPIn) Close()            { (*i.connection).Close(); i.isOpen = false; print("TCPUDPInClose") }
func (i *TCPUDPIn) IsOpen() bool      { return i.isOpen }
func (i *TCPUDPIn) Name() string      { return "TCPUDPIn" }

/**
	//OUTPUT DEFINITIONS
**/
func OutputChannel(proto, path, port string) chan AnyIn {
	inputs := make(chan AnyIn)
	switch proto {
	/*
		case "OutputType":
			//Do some init
			go func(){ //Start a routine which waits for AnyIn sent back on my chan
				for {  //always
					go func(in){	//Start a handler
										//Which reads the input to our init'd output stream
					}(<-inputs) 	//When we get an input stream
				}
			}()
	*/
	case "file":
		go func() {
			File := FileOut{path: path}
			File.Init()
			defer File.Close()
			for {
				go func(input AnyIn) {
					print("\nDebug: got session, pipe to file")
					PipeIO(input, &File)
				}(<-inputs)
			}
		}()
	case "std":
		go func() {
			stdout := StdOut{output: os.Stdout}
			stdout.Init()

			for {
				go func(input AnyIn) {
					PipeIO(input, &stdout)
				}(<-inputs)
			}
		}()
	case "tcp", "udp", "unix":
		go func() {
			//Share connection
			target, err := net.DialTimeout(proto, path+":"+port, time.Duration(900)*time.Millisecond)
			tcpudpout := TcpUdpOut{connection: &target}
			tcpudpout.Init()
			for {
				go func(input AnyIn) {
					if err != nil {
						if dbg > 0 {
							print("TcpUdpOutConnect:" + err.Error() + "\n")
						}
						input.Write([]byte(fmt.Sprintf("5XX:%s", err.Error())))
						input.Close()
						return
					}

					PipeIO(input, &tcpudpout)
				}(<-inputs)
			}
		}()

	}
	return inputs
}

type AnyOut interface {
	Init()                     //Initial setup
	Read(*[]byte) (int, error) //Optionally blocking call to get a reply chunk
	Write([]byte)              //Send to output
	Close()                    //Uninit
	IsOpen() bool              //Check if running
	Name() string
}

//FILE
type FileOut struct {
	path string
	file *os.File
	open bool
}

func (o *FileOut) Init() {
	if file, err := os.OpenFile(o.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
		if os.IsNotExist(err) {
			folder := path.Dir(o.path)
			if err := os.MkdirAll(folder, os.FileMode(0666)); err != nil {
				fmt.Printf("FileInitCreateFolderErr:%v: %v\n", folder, err)
				return
			}
			newfile, err := os.Create(o.path)
			if err != nil {
				fmt.Printf("FileInitCreateFileErr:%v: %v\n", o.path, err)
				return
			}
			o.file = newfile
		} else if os.IsPermission(err) {
			fmt.Printf("Do not have permission to write to %v: %v\n", o.path, err)
			return
		} else {
			fmt.Printf("FileInitErr: %v\n", err)
			return
		}
	} else {
		o.file = file
	}
	o.open = true
}
func (o *FileOut) Read(*[]byte) (int, error) { return 0, nil } //By default, do not allow reading back from output file
func (o *FileOut) Write(data []byte) {
	if len(data) == 0 {
		return
	}
	if _, err := o.file.Write(data); err != nil {
		fmt.Printf("FileWriteErr: %v\n", err)
	}
}
func (o *FileOut) Close()       { o.file.Close(); o.open = false }
func (o *FileOut) IsOpen() bool { return o.open }
func (o *FileOut) Name() string { return "FileOut" }

//STDOUT
type StdOut struct {
	output *os.File
	open   bool
}

func (o *StdOut) Init()                          { o.output = os.Stdout; o.open = true }
func (o *StdOut) Read(data *[]byte) (int, error) { return 0, nil } //By default, do not allow reading back from output file
func (o *StdOut) Write(data []byte)              { o.output.Write(data) }
func (o *StdOut) Close()                         { o.open = false }
func (o *StdOut) IsOpen() bool                   { return o.open }
func (o *StdOut) Name() string                   { return "StdOut" }

//TCPUDP
type TcpUdpOut struct {
	connection *net.Conn
	isOpen     bool
}

func (o *TcpUdpOut) Init()                          { o.isOpen = true }
func (o *TcpUdpOut) Read(data *[]byte) (int, error) { return (*o.connection).Read(*data) }
func (o *TcpUdpOut) Write(data []byte)              { (*o.connection).Write(data) }
func (o *TcpUdpOut) Close()                         { (*o.connection).Close(); o.isOpen = false; print("TCPUDPOutClose") }
func (o *TcpUdpOut) IsOpen() bool                   { return o.isOpen }
func (o *TcpUdpOut) Name() string                   { return "TCPUDPOut" }

/**
	//FUNCTIONS
**/

// PipeIO creates a full-duplex pipe between any two AnyIO.
func PipeIO(ioIn, ioOut AnyIO) {
	print("\nDebug: piping " + ioIn.Name() + "→" + ioOut.Name())
	IIn, IOut := chanFromAnyIO(ioIn)
	OIn, OOut := chanFromAnyIO(ioOut)
	// close := func() {
	// 	ioIn.Close()
	// 	ioOut.Close()
	// }
	print("\nDebug: piped")
	go func() {
		for {
			select {
			case data := <-*OOut:
				print("\nDebug: output " + string(data))
				if data != nil {
					// print("\nDebug: closed output " + string(data))
					// close()
					// return
					*IIn <- data
				}

			case data := <-*IOut:
				if data != nil { //&& len(data) > 0 {
					*OIn <- data
					print("\nDebug: input " + string(data))
					// print("\nDebug: closed input " + string(data))
					// close()
					// return
				}

			}
		}
	}()
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

type AnyIO interface {
	Init()
	Read(*[]byte) (int, error)
	Write([]byte)
	Close()
	IsOpen() bool
	Name() string
}

func chanFromAnyIO(io AnyIO) (*chan []byte, *chan []byte) {
	in := make(chan []byte)
	out := make(chan []byte)

	go func() {
		b := make([]byte, 1024)
		for {
			n, err := io.Read(&b)
			if n > 0 {
				res := make([]byte, n)
				// Copy the buffer so it doesn't get changed while read by the recipient.
				copy(res, b[:n])

				print("\nDebug: Did a read from " + io.Name() + ":" + string(b))

				out <- res
			}
			if err != nil {
				print("\nChan(" + io.Name() + ")ReadErr:" + err.Error())
				break
			}
			if !io.IsOpen() {
				print("\nDebug: Closing !open " + io.Name())
				close(in)
				close(out)
				return
			}
		}
	}()
	go func() {
		for data := range in {
			io.Write(data)
		}
	}()

	return &in, &out
}
