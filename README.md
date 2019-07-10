## Note

This project has been more or less abandoned in favor of the much more complete https://github.com/craSH/socat

It was a fun learning experience, and the code will be left here for anyone wishing to use it or PR for a new direction or useful addition.


### TLDR

Take data from one port and put it into another. Useful for making a port/host on the local network explicitly available to a specific external port. I use it for setting up docker containers that I might want to leave running but keep the port closed. 

If you just want to forward some ports on your docker container, use the stable prebuilt binary which is from circa 8a9e801421a232fde49be8f4f1cf30c85f8b0a06. If that fails, I recommend you go to that commit and go build fwd.go from there, because as of writing this there are some changes and bugs in the works.

Stable:

``` 
    wget https://raw.githubusercontent.com/CreativeCactus/TCPChan/master/fwd.old
    chmod +x fwd.old
    ./fwd.old  tcp 0.0.0.0:80 tcp <docker ip addr>:8080
```

Latest:

```
    wget https://raw.githubusercontent.com/CreativeCactus/TCPChan/master/fwd
    chmod +x fwd
    ./fwd  --src=tcp:0.0.0.0:80 --dst=tcp:<docker ip addr>:8080 &> /dev/null &
```

# TCPChan ←→

<img src="https://raw.githubusercontent.com/CreativeCactus/TCPChan/master/cat.png" alt="cat" style="height:150px; width:250px; right: 0px; position:absolute;"></img>

A little tool for forwarding TCP traffic. Notably useful for port-forwarding docker containers on a remote host.

# How do I use this?

``` go build fwd.go ```

``` ./fwd # help ```

``` ./fwd tcp 127.0.0.1:80 google.com:80 ```

``` ./fwd tcp 0.0.0.0:80 udp 10.0.1.100:8080 ```

Note! You need to have uuidgen available. ``` apt-get install uuid-runtime ``` should do it

Supported protocols are 
 - tcp
 - udp
 - unix (socket file, untested)
 
# Update in progress
 
I am working to add a few different protocols and features to make this more useful.

## Protocols

 - file - Serve a file or write to a file
 - std  - STDIO
 - unix - socket file
 - udp/tcp - precisely as one might expect

## Features

In addition to accepting arguments on --src and --dst to avoid confusion, I will be adding --fnc with the option to encode/decode streams. This may not play well with multiplexing, though I'd imagine ROT13 and line by line would be fine.




If you run the latest build with -? you should see something like the following:
 
```
TCPChan fwd v20160826
Usage: fwd [OPTIONS] -src=[src] -dst=[dst]
                -src    the proto:path:port to listen on
                -dst    the proto:path:port to forward to
                -fnc    beta. Define an encoding/decoding behaviour
                -v      verbosity. default lvl 1. If proto out is std, default 0.

                proto   one of file/std/unix/udp/tcp
                                file and unix do not require a port
                                std does not require a path or port
                                * if a file has a port value it will be used as the buffer size, default 128, 0=newline delimited 
                path    in the case of file or unix, a file or socket respectively
                                in the case of upd or tcp, an IP address or hostname
                port    the port of the IP or hostname

Examples
                fwd -src=tcp:127.0.0.1:3000 -dst=file:/tmp/output
                fwd -src=unix:/tmp/in.sock  -dst=udp:10.0.0.1:999
                fwd -src=stdin -dst=stdout
Note: options are not heavily checked
Use fwd --help for more usage
```

