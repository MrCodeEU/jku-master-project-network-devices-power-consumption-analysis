# Idea
We will group phases to make the testing easier:

## First gorup
1. 3min with nothing (and wifi off (both radios))
2. 2 min per cabel

# Second group
1. keep all cabels connected 
2. attach 1 usb device after the other (debendig onusb port count e.g. 0 for alcatel)

# Third group
1. keep all cabels and usb devices conencted
2. turn on 2.4 radio
3. turn on 5 radio (not applicable to alcatel)

# Fourth group
1. again everything as it was from the group before
2. add 2.4 and 5 ghz client in idle (ideall one after the other so wait like 2 minutes in between)

# Fifth group
1. with clients still connected and everything else
2. start wifi load between clien 24. and 5 ghz

# Last gorup
1. start wifi load again with everything connected
2. Start full ethernet load on all ports
3. stop load tests and also meassure post load test power
4. Maybe start disconnecting everythin one after the other in reverse order? (but that is not really interesting data)

## Now repeat the above for all devices

## Then repeat again with power saving settings enable (nothing special just power saving mode / eco mode if available) (i think only asus and fritzbox need to check?)

# Tests
## alcatel
1. only 2 lan port wifi off 
2. group 2 usb skip
3. group wifi only turn on 2.4
4. add only 2.4 ghz client + a second 2.4 ghz client
5. wifi load (only a certain amount sustained full load then goes back down?) (average 17.5 mbits both directions)
6. wifi + ethernet load with all of before. (interestingly wifi performace quite a drop and ethernet ios fully there? without much more powerdraw?) (wifi average 8.45 Mbits)

## asus
1. 4 lan cables (ignore dedicated wan port for this as i dont have 5th lan port)
2. 2 usb ports
3. turn on 2.4 and then 5 ghz
4. connect 1 phone 2.4 and one 5 (no reall effect on powerdraw)
5. wifi load between the two phones. (appears to max out wifi 2.4 nicelly 147 mbits average) (also reflected in the constant powerdraw)
6. wifi load again + ethernet load (average wifi speed still arounf 140 mbits and ood 4 gig throughput with slight drops here and there)

## huawei
1. 4 lan cabels wifi off  (2 tel ports are ignored i dont have kabels or hardware for this)
2. 1 single usb device
3. turn on 2.4 and 5
4. add clients 2.4 and 5 (again no difference in power)
5. wifi load between two phones (similar ot alcatel drop in speed in correlation to the powerdraw drop average of 58.5 mbit with beginnig closer to 85 and at the end more like 35?)
6. wifi and ethernet load (ethernet is full 4 gigs and wifi drops form arounf 70-90 to 40-65 mbits but powerdraw not really more? so priority on lan?) (but during ethernet load occasionally burst to 80 again thertefore the average is 70 mbits still.)

## fritzbox
1. wifi off 4 lan cabels ( no dsl or fon)
2. 1 usb on the side
3. turn on wifi all at once (no other setting/option found?) (it looked like there are two distinct power steps question is is that because fritzbox turned on on radio and then the next or is that because of the meassuerment of 15 secoonds from the powermeter caching during rising?)
4. add client for 2.4 and 5 (interesting here we can meassure the clients conencting small but meassurable and stable power increase)
5. wifi load test between two phones (impressive 194 mbits average very stable and relativelly small powerdraw i think but much more stable networkspeed compared to the powerdraw that was quite jumpy)
6. wifi + ethernet load (wifi spee droped from around 200 to 130-180 very jumpy) (also one of the ethernet ports (ethernet 7) was completlly broken as it reports speeds at the level of the full nic which is likelly due to every frame beeing dropped and it constantlly resends them?) (The other ports are at aroud 450 which is intereasting) (also the powerdraw is very much al over the place) (average wifi speed at 163mbits)

## power saving mode
# asus
only tx power of wifi radios (use lowest setting from wifi onwards the cables and usb dont change)

# alcatel
nothing!

# huawei 
TX Power for wifi (use lowest setting and repeat from wifi onwards)

# fritzbox
yes deticated eco mode!
(fritzbox webui comment: Diese Einstellung verringert die Leistung von WLAN sowie der LAN- und USB-Schnittstellen zugunsten des geringeren Strombedarfs. Die Helligkeit der LEDs wird auf „schwach” gesetzt.)

# some caviates
since test are split there will likelly be quite the jumps beweent the groups end and beginnings as th epower draw is not always the exact same but we will have to seen once we combine the data (for now no major jumps sometimes a few small jumps but overall very consistant)

# fritzbox 
full run agaon but with eco mode enabled.
1. 4 lan cables with wifi off ( i think smaller phy power jumps?)
2. usb port (much larger jump then ethernet this time)
3. turn on wifi (again both together) (looks like quit the jump? need to compare it directlly!)
4. again 2.4 and 5 clients connect (not really meassurable difference? needs comparison)
5. wifi load test (much more fluctuation in speed between 150-225mbits results in an average of 185mbits)
6. again wifi and ethernat load full blast (again same problem with ethernet 7 why? is this a priority thing with fritzbox? the same setup worked for asus and huawei? changfed the cabels and rerun the test this time it is the interface Ethernet so it is a port on the fritzbox?) (interestinglly this time average wifi speed stayed at 180mbits)

# asus
setup with all cabels and usb connected but wifi turned off + already set to lowes tx power (power saving)
3. turn on 2.4 then 5.
4. connect 2.4 and 5 clients (no effect?)
5. wifi load between phones (still quite some power draw. needs comparison. average 163mbits)
6. wifi + ethernet (since only wifi powr changed i dont think this has that much value but lets see: hmm better average wifi throughput and full 4gig ethernet. average: 192mbits. this is a bit strange? but tests where done directly one after the other?)

# huawei
setup with all cabels and usb connected but wifi turned off + already set to lowes tx power (20%)
3. turn on 2.4 and 5 (looks like less need to compare)
4. connect 2.4 and 5 (no effect? or maybe minimal for 5 need detailed analysis)
5. wifi load test (quite aa lot of power draw need comparison as for speed an average of 98.3mbits)
6. wifi + ethernet load (again does this make sense? lets see. wifi not affected by ethernet average: 95.4mbits)

# one interesting sidenote 
huawei and asus devices bacame quite warm to the touch under group 6. alcatel and fritzbox not.
it would be nice to view the cpu temps but that only works for asus and fritzbox and therefore i will ignore that for now.