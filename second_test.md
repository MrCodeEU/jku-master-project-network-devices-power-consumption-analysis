# second test is actually test 5 in the report

Run the same test as test 1 but DUT is huawei and connection goes through asus router to huawei which is in bridge mode.
Now we meassure power of both devices connected to one power strip.

## Why these two devices?
huawei router was provided by energie AG and the asus router has a dedicated WAN port for better connectivity i guess. It just feels like it makes more sense?

## Setup
Now setup devices correctlly such that traffig target can be the huawei mode in bridge mode while interfaces are ehternet ports connected to the load generation pc. 

In asus router webui set static route for 192.168.18.1/24 to go to WAN intrface. and a NAT to return to the routers wan ip address again.

static route on router route add -host 192.168.18.1 dev eth0

plus a static route on the load gen pc to use the asus router route -p add 192.168.18.1 mask 255.255.255.255 192.168.51.1 metric 1