# TCPChan

![cat](cat.png =250x150)

A little tool for forwarding TCP traffic. Notably useful for port-forwarding docker containers on a remote host.

<style>img[alt=cat] { width: 200px; }</style>

# How use?

Glad asked!

``` go build fwd.go ```

``` ./fwd # help ```

``` ./fwd tcp 127.0.0.1:80 google.com:80 ```

``` ./fwd tcp 0.0.0.0:80 udp 10.0.1.100:8080 ```

Supported protocols are 
 - tcp
 - udp
 - unix (socket file, untested)

