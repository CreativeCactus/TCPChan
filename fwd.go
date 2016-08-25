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
)

//Print warnings to stdout, unless we are piping to stdout
var dbg = true

func main() {
	// Set up the configuration
	src_flag := "tcp:127.0.0.1:81"
	dst_flag := "tcp:127.0.0.1:80"
	flag.StringVar(&src_flag, "src", src_flag, "Where we will listen for connections, eg. udp:127.0.0.1:22")
	flag.StringVar(&dst_flag, "dst", dst_flag, "Where we will forward data to, eg. unix:/tmp/pipe.sock")
	helpopt := flag.Bool("?", false, "Show help")
	flag.Parse()
	help := *helpopt || *flag.Bool("help", false, "Show help") || *flag.Bool("h", false, "Show help")
	if help {
		fmt.Print(`
Usage: fwd [OPTIONS] -src=[src] -dst=[dst]
		-src 	the proto:path:port to listen on
		-dst 	the proto:path:port to forward to
		
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
	var split [2]string
	copy(split[:], strings.SplitN(src_flag, ":", 2))
	srcProto := split[0]
	srcIpport := split[1]
	copy(split[:], strings.SplitN(dst_flag, ":", 2))
	dstProto := split[0]
	dstIpport := split[1]

	fmt.Printf("%v %v → %v %v\n",
		srcProto, srcIpport,
		dstProto, dstIpport)

	// Create streams
	split = [2]string{}
	copy(split[:], strings.SplitN(srcIpport, ":", 2))
	fmt.Printf("dbg:%v\n", split)
	input := InputChannel(srcProto, split[0], split[1])

	split = [2]string{}
	copy(split[:], strings.SplitN(dstIpport, ":", 2))
	fmt.Printf("dbg:%v\n", split)
	output := OutputChannel(dstProto, split[0], split[1])

	//serve intput to output
	for io := range input {
		output <- io
		defer io.Close()
	}
	/*
		split = [2]string{}
		copy(split[:], strings.SplitN(srcIpport, ":", 2))
		switch srcProto {
		case "file":
			bufsize := 0
			if bufsize, err := strconv.Atoi(split[1]); err != nil || bufsize == 0 {
				bufsize = 128
			}
			input = &FileIn{path: split[0], bufsize: bufsize}
		case "std":
			bufsize := 0
			if bufsize, err := strconv.Atoi(split[1]); err != nil || bufsize == 0 {
				bufsize = 128
			}
			input = &StdIn{bufsize: bufsize}

		}
		input.Init()
		defer input.Close()

		split = [2]string{}
		copy(split[:], strings.SplitN(dstIpport, ":", 2))
		switch dstProto {
		case "file":
			output = &FileOut{path: split[0]}
		case "std":
			output = &StdOut{}
		}
		output.Init()
		defer output.Close()
		////////////////////////////
		//TODO how will we handle multiplexed connections with files?

		// Blocking Server
		go func() {
			for input.IsOpen() && output.IsOpen() {
				output.Write(input.Read())
			}
		}()
		go func() {
			for input.IsOpen() && output.IsOpen() {
				input.Reply(output.Read())
			}
		}()

		for <-input {
		}
	*/
	/*
		switch srcProto {
		case "file":
			target, err := net.DialTimeout(dstProto, dstIpport, 500000)
			if err != nil {
				if dbg {
					fmt.Printf("5XX:%s", err.Error())
				}
				return
			}

		case "stdin":

			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				fmt.Println(scanner.Text()) // Println will add back the final '\n'
			}
			if err := scanner.Err(); err != nil {
				fmt.Fprintln(os.Stderr, "reading standard input:", err)
			}
		case "stdout":
			panic("Cannot read in from stdout")
		default:
			session, _ := net.Listen(srcProto, srcIpport)
			for {
				conn, _ := session.Accept()
				go handleComms(conn, dstProto, dstIpport)
			}
		}
	*/
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
		bufsize := 0
		if bufsize, err := strconv.Atoi(port); err != nil || bufsize == 0 {
			bufsize = 128
		}
		inputs <- &StdIn{bufsize: bufsize}
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
	input   *os.File
	buffer  []byte
	bufsize int
	open    bool
}

func (i *StdIn) Init() { //Allow 0 for newline delimited?
	i.input = os.Stdin
	i.buffer = make([]byte, i.bufsize)
	i.open = true
}
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
			target, err := net.DialTimeout(proto, path+":"+port, 500000)
			tcpudpout := TcpUdpOut{connection: &target}
			tcpudpout.Init()
			for {
				go func(input AnyIn) {
					if err != nil {
						if dbg {
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
	if file, err := os.Open(o.path); err != nil {
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

func (o *StdOut) Init()                          { o.output = os.Stdout; dbg = true; o.open = true }
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
