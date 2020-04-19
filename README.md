MeshMembers
===========
MeshMembers is a wrapper around
[meshlist](https://github.com/hashicorp/memberlist).  It keeps track of which
nodes are up in a mesh network.

The original concept involved having a pool of redirectors at the ready for
situations where tunneling network traffic is a good idea.

Quickstart
----------
The first node in the mesh just needs to listen

```
$ ./meshmembers
2020/04/18 23:15:46 Listen address: 0.0.0.0
2020/04/18 23:15:46 External address: 192.0.2.24
2020/04/18 23:15:46 Port: 7887
2020/04/18 23:15:46 Starting mesh listeners
2020/04/18 23:15:46 This node: openbsd-amd64-48:13:44:81:38:42-c24uw12mmz87 (192.0.2.24:7887)
```

Subsequent connect to at least one node already in the network
```
$ ./meshmembers -peers 198.51.100.3:7887,198.51.100.49:7887
2020/04/18 23:17:24 Listen address: 0.0.0.0
2020/04/18 23:17:24 External address: 203.0.113.248
2020/04/18 23:17:24 Port: 7887
2020/04/18 23:17:24 Starting mesh listeners
2020/04/18 23:17:24 This node: openbsd-amd64-82:c6:91:a3:84:fe-c24uxaeeh5te (203.0.113.248:7887)
2020/04/18 23:17:24 Initial peer list: [meshtest.ga:7887]
2020/04/18 23:17:25 [Join] linux-amd64-9e:18:69:b6:df:74-c24tenesruxe (198.51.100.3:7887)
2020/04/18 23:17:25 [Join] linux-amd64-36:11:0e:72:35:63-c24s3oy2gpqy (198.51.100.49:7887)
2020/04/18 23:17:25 [Join] linux-amd64-22:cb:6a:05:c8:fc-c24ryneo8xic ([2001:db8:400:d1::8c9:2001]:7887)
2020/04/18 23:17:25 [Join] linux-amd64-92:b1:b4:8c:3d:6d-c24tmiwtnb7c (100.64.3.41:7887)
2020/04/18 23:17:25 [Join] linux-amd64-62:7f:3b:ba:41:63-c24tmfrtznfu (100.64.5.132:7887)
2020/04/18 23:15:46 [Join] openbsd-amd64-48:13:44:81:38:42-c24uw12mmz87 (192.0.2.24:7887)
2020/04/18 23:17:25 Connected to 2 initial peers
```

Building
--------
```sh
go get github.com/magisterquis/meshmembers
```

Initial Peer
------------
At least one other member of the mesh must be know ahead of time to join an
existing mesh, and must be specified with `-peers`.  If none are given,
MeshMembers will listen for incoming connections.

Secret
------
There is a secret (`-secret`) shared amongst every node in the mesh.  This
provides a weak form of authentication, but should not be relied upon for any
real form of security.

Node Name
---------
Each node in the mesh must have a unique name.  By default a name similar to
`openbsd-amd64-de:ad:be:ef:ca:fe-c24tmewonb7c` is generated based on the
platform, MAC address, and current time.  A name may be set with `-name`.

Addresses
---------
The address on which MeshMembers listens for new connections (`-listen`) need
not be the same as the address advertised for incoming connections
(`-external`).  This is helpful when there's inbound natting (e.g. EC2).  The
same port (`-port`) is used for both and is used for both TCP and UDP.  The
port does not have to be the same for all members of the mesh.

Local Clients
-------------
Aside from the logging done by meshmebers to stdout, MeshMembers can send a
list of nodes it knows about via a Unix socket (`-socket`).  New nodes joining
and leaving the network will also be reported via the socket.

If MeshMembers is started with `-socket /tmp/.meshmembers.sock`:
```
$ nc -U /tmp/.meshmembers.sock
Current nodes in mesh: 5
linux-amd64-9e:18:69:b6:df:74-c24tenesruxe (198.51.100.3:7887)
linux-amd64-36:11:0e:72:35:63-c24s3oy2gpqy (198.51.100.49:7887)
linux-amd64-22:cb:6a:05:c8:fc-c24ryneo8xic ([2001:db8:400:d1::8c9:2001]:7887)
linux-amd64-92:b1:b4:8c:3d:6d-c24tmiwtnb7c (100.64.3.41:7887)
linux-amd64-62:7f:3b:ba:41:63-c24tmfrtznfu (100.64.5.132:7887)
2020/04/18 23:15:46 [Join] openbsd-amd64-48:13:44:81:38:42-c24uw12mmz87 (192.0.2.24:7887)
```

This shows a mesh which had 5 nodes when the connection was initially made to
the unix socket plus another which joined afterwards.

SSH Tunnels
-----------
The below perl one-liner is useful for tunneling through a three of the boxes
in the mesh.  It assumes an SSH config similar to
```
ControlMaster no
User hop
StrictHostKeyChecking accept-new
DynamicForward 5555
```

###One-liner:

```perl -e '@l=grep{/\(.*\)$/}split/\n/,`nc -Uw1 /tmp/.meshmembers.sock | sort -R`;s/^.*\(|:\d+\)//g for@l;die"Not enough nodes"if 3>$#l;$t=pop@l;$#l=2;$c=sprintf"ssh -v -N -F ~/.ssh/mmconfig -J %s %s",join(",",@l),$t;print$c,$/;exec$c'
```

The tweakable bits are
- `/tmp/.meshmembers.sock` - MeshMembers Unix socket
- `3>$#l` (specifically 3) - The number of hosts to hop through before the
   final host. The `2` in `$#l=2` needs to be changes to one fewer than the
   number of hops
- `~/.ssh/mmconfig` - SSH config file

On second thought, this would have been better as a script.

Testing
-------
For ease of testing, a skeleton of a
[cloud-init](https://cloudinit.readthedocs.io/en/latest/) file is included
in this repo an [`cloud-init.yaml`](./cloud-init.yaml).  A MeshMembers binary
will need to be built and hosted somewhere and an initial peer started before
using it.  The config makes a user named `hop` with an SSH key meant to be used
for tunneling SSH.  

A script, [`docreate.sh`](./docreate.sh), which wraps DigitalOcean's `doctl` is
provided to help set up a test mesh in DigitalOcean.

