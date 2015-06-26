# kone
Simple CLI for getting simple status info from multiple machines.

## Usage
```
./kone -data <data file path> -key <key file path> [-pass <key password file path>]
```

Data file must contain a JSON array of remote machines in following format:
```
[
{
  "name": "machine display name",
  "user": "username",
  "host": "127.0.0.1",
  "port": "123",
  "load1": {"warning": 0.9, "error": 1},
  ...
},
...
]
```

Each entry for a machine can contain error and warning levels for following parameters:
* load1 - 1 minute load average
* load5 - 5 minute load average
* load15 - 15 minute load average
* cpu - cpu utilization in percentage (`top -b -n2 | grep "Cpu(s)"|tail -n 1 | awk '{print $2 + $4}'`)
* free - memory usage in percentage (`free | grep Mem | awk '{print ($3-$6-$7)/$2}'`)
* storage - disk usage in percentage (`df / | grep '/' | awk '{print $5}'`)
* inode - inode usage in percentage (`df -i / | grep '/' | awk '{print $5}'`)
* conns - connections count (`netstat -ant | awk '{print $5}' | uniq -u | wc -l`)

Key file is for example ~/.ssh/id_rsa, and password file is a file that contains only the password for sha key, if the key has been password protected.


## Screenshot
![Screenshot](/../screenshot/output.gif?raw=true "Screenshot")

##Acknowledgements
Written using [termbox-go](https://github.com/nsf/termbox-go) library
