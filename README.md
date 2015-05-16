# kone
Simple CLI for getting simple status info from multiple machines.

## Usage
```
./kone -data <data file path> -key <key file path> [-pass <key password file path>]
```

Data file must contain list of remote machines in following format:
```
unique remote machine name 1, username, ip-address, port 
unique remote machine name 2, username, ip-address, port 
...
```
Key file is for example ~/.ssh/id_rsa, and password file is a file that contains only the password for sha key, if it has been password protected.


## Screenshot
![Screenshot](/../screenshot/output.gif?raw=true "Screenshot")

##Acknowledgements
Written using [termbox-go](https://github.com/nsf/termbox-go) library
