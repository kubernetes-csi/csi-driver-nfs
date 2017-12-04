# CSI NFS driver

## Usage:

### Start NFS driver
```
$ sudo ../_output/nfsdriver --endpoint tcp://127.0.0.1:10000 --nodeid CSINode
```

### Test using csc
Get ```csc``` tool from https://github.com/chakri-nelluri/gocsi/tree/master/csc

#### Get plugin info
```
$ csc identity plugininfo --endpoint tcp://127.0.0.1:10000
"NFS"	"0.1.0"
```

### Get supported versions
```
$ csc identity supportedversions --endpoint tcp://127.0.0.1:10000
0.1.0
```

#### NodePublish a volume
```
$ export NFS_SERVER="Your Server IP (Ex: 10.10.10.10)"
$ export NFS_SHARE="Your NFS share"
$ csc node publishvolume --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs --attrib server=$NFS_SERVER --attrib exportPath=$NFS_SHARE nfstestvol
nfstestvol
```

#### NodeUnpublish a volume
```
$ csc node unpublishvolume --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs nfstestvol
nfstestvol
```

#### Get NodeID
```
$ csc node getid --endpoint tcp://127.0.0.1:10000
CSINode
```

