# Static routing
192.168.50.100 is beeing routed directlly over ethernet interface 6 via:
```route add 192.168.50.100 mask 255.255.255.255 192.168.50.168 if 16 metric 1```
All other 192.168.50.* traffic is routed via ethernet 7 for now 
Later on if we use all 4 ports of the nic we will use the motherboard port and adjust the routing table accordingly!

Test-NetConnection -ComputerName 192.168.50.80 -Port 80

ComputerName     : 192.168.50.80
RemoteAddress    : 192.168.50.80
RemotePort       : 80
InterfaceAlias   : Ethernet 7
SourceAddress    : 192.168.50.169
TcpTestSucceeded : True

Test-NetConnection -ComputerName 192.168.50.100 -Port 80

ComputerName     : 192.168.50.100
RemoteAddress    : 192.168.50.100
RemotePort       : 80
InterfaceAlias   : Ethernet 6
SourceAddress    : 192.168.50.168
TcpTestSucceeded : True

# stress testing code:
## Build the program first
go build -o stress.exe main.go

# === UDP Tests (Maximum Throughput) ===

## Test 1: Maximum gigabit saturation
.\stress.exe -target 192.168.50.100 -proto udp -port 80 -workers 16 -size 1400 -duration 60 -interface 192.168.50.168

## Test 2: High packet rate stress test (only 650?)
.\stress.exe -target 192.168.50.100 -proto udp -port 80 -workers 20 -size 512 -duration 30 -interface 192.168.50.168

## Test 3: Maximum UDP packet size (no fragmentation)
.\stress.exe -target 192.168.50.100 -proto udp -port 80 -workers 16 -size 1472 -duration 60 -interface 192.168.50.168

## Test 4: Extreme packet rate (tiny packets) (jumps alot between 100 and 1000)
.\stress.exe -target 192.168.50.100 -proto udp -port 80 -workers 24 -size 256 -duration 30 -interface 192.168.50.168

## === TCP Tests ===

## Test 5: TCP to web interface (realistic load) (error writing?)
.\stress.exe -target 192.168.50.100 -proto tcp -port 80 -workers 8 -size 8192 -duration 30 -interface 192.168.50.168

## Test 6: Heavy TCP load (forcibly closed by remote)
.\stress.exe -target 192.168.50.100 -proto tcp -port 80 -workers 16 -size 16384 -duration 60 -interface 192.168.50.168

## Test 7: Many concurrent connections (forcibly closed)
.\stress.exe -target 192.168.50.100 -proto tcp -port 80 -workers 32 -size 4096 -duration 30 -interface 192.168.50.168