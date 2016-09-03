### TLDR

If you just want to forward some ports on your docker container, use the prebuilt binary which is from circa 8a9e801421a232fde49be8f4f1cf30c85f8b0a06. If that fails, I recommend you go to that commit and go build fwd.go from there, because as of writing this there are some changes and bugs in the works.

``` 
    wget https://raw.githubusercontent.com/CreativeCactus/TCPChan/master/fwd
    chmod +x fwd
    ./fwd  tcp 0.0.0.0:80 tcp <docker ip addr>:8080
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

