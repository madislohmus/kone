# kone
Simple CLI for getting simple status info from multiple machines.

## Usage
```
./kone -data <data file path> -key <key file path> [-pass <key password file path>] [-cmd <custom commands file path>]
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
`name`, `user`, `host` and `port` are mandatory fields.

Each entry for a machine can contain error and warning levels for following parameters:
* load1 - 1 minute load average (`cat /proc/loadavg`)
* load5 - 5 minute load average (`cat /proc/loadavg`)
* load15 - 15 minute load average (`cat /proc/loadavg`)
* cpu - cpu utilization in percentage (`top -b -n2 | grep "Cpu(s)"|tail -n 1 | awk '{print $2 + $4}'`)
* free - memory usage in percentage (`free | grep Mem | awk '{print ($3-$6-$7)/$2}'`)
* storage - disk usage in percentage (`df / | grep '/' | awk '{print $5}'`)
* inode - inode usage in percentage (`df -i / | grep '/' | awk '{print $5}'`)
* conns - connections count (`netstat -ant | awk '{print $5}' | uniq -u | wc -l`)

Key file is for example ~/.ssh/id_rsa, and password file is a file that contains only the password for sha key, if the key has been password protected. Custom commands are mapped to F1-F12. A file can be passed as a parameter that contains custom commands with following syntax:
```
F1=cmd1
F2=cmd2
...
```

The program connects to machines through ssh. At every 5 minutes (or when invoked manually) it runs the above mentioned commands on a machine to gather status information.

Keys:
* `f` - forces re-connect to machines
* `s` - only warning and error information is displayed.
* `i` - machine IP is shown instead of its name
* `ctrl + r` -  reload status info for currently selected machine
* `ctrl + a` - reload status info for all machines
* `ctrl + f` - search (machine name / IP)
* `Enter` - open shell to selected machine
* `F1-12` - open a shell to the selected machine and issue the command (if any) assigned to the F-key.
* `Esc` - exit search / program

## Screenshot
![Screenshot](/../screenshot/output.gif?raw=true "Screenshot")

## Acknowledgements
Written using [termbox-go](https://github.com/nsf/termbox-go) library
