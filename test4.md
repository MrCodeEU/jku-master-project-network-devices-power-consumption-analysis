# Setup

as per document + each test 2.4 or 5 has only the needed radio enable to better isolate the readios power efficiency

# asus
2.4 close test has strange drop in speed and also power usge?
5 close

# huawei
after a lot of trial and error i have figured out that the Huawei router is isolating the lan and wifi clients and thereofre i am unable to get a connection fromt the pc to the wifi client (e.g. ping only works phone to pc not pc to phone!!) WHile this is quite annoying i will instead run the tests manually and do it between two wifi clients instead!
hHile this does put a bit more stress on the radio it should be neglegible as they are 2x2 or more and isntead of 2x2 for one devie the split it 1x1 per 1 device???????? (not sure about that need more research!)

repeat 2.4 and 5 wall 8data loss and sever interrupted!
better now. (but still annoying that seperate speed txt file with extrac processing steps to average 5 points to one)

# alcatel
only 2.4 ghz tests possible!

# fritzbox 
cannot turn off individuall radios therefore tests runn with both activated!
Again isolation between wifi and LAN. But this time i was able to connect load gen pc via wifi and test phone via wifi and therefore have proper data 

# some stuff
1. wifi has much higher power draw then lan
2. channel with very important for speed asus the only one with 160? or 80mhz mush faster
3. distance not that big of an impact on power draw but definetlly speed and stability high variablity
4. anything else? huawei much slower than expected + also much more unstable. Alcatel very stable and pretty fast compated to what i was expecting. as the wifi test maxed out the lan port (renun with wifi then we also have 4 tests)

# alcatel again

this time load gen pc is connected via wifi as well to check max wifi spee