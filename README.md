# MaxMind DB Reader for Go #

[![Build Status](https://travis-ci.org/oschwald/maxminddb-golang.png?branch=master)](https://travis-ci.org/oschwald/maxminddb-golang)

This is a Go reader for the MaxMind DB format. This can be used to read
[GeoLite2](http://dev.maxmind.com/geoip/geoip2/geolite2/) and
[GeoIP2](http://www.maxmind.com/en/geolocation_landing) databases.

This is not an official MaxMind API.

## Status ##

This API is functional, but still needs quite a bit of work to be ready for
production use. Here are some things that need to be done:

* `Unmarshal` does not currently work with `uint128` data from the database.
* The metadata needs to be put into a struct. The current type assertions
  are gross.
* Docs need to be written.
* The code should be made idiomatic.
* Verify that arrays/slices are being passed around correctly.
* Although IPv4 addresses work, the code to speed up IPv4 lookups is not
  working as ParseIP always seems to return 16 bytes.
* Error handling should be improved.

## Unmarshal Example ##

```go

package main

import (
    "fmt"
    "log"
    "github.com/oschwald/maxminddb-golang"
    "geoip2"
    "net"
)

func main() {
    db, err := maxminddb.Open("GeoLite2-City.mmdb")
    if err != nil {
        log.Fatal(err)
    }
    ip := net.ParseIP("1.1.1.1")

    var record geoip2.City // Or any appropriate struct
    err := db.Unmarshal(ip, record)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(record)
    db.Close()
}

```

## Lookup Example ##

```go

package main

import (
    "fmt"
    "log"
    "github.com/oschwald/maxminddb-golang"
    "net"
)

func main() {
    db, err := maxminddb.Open("GeoLite2-City.mmdb")
    if err != nil {
        log.Fatal(err)
    }
    ip := net.ParseIP("1.1.1.1")
    record, err := db.Lookup(ip)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(record)
    db.Close()
}

```

## Contributing ##

Contributions welcome! Please fork the repository and open a pull request
with your changes.

## License ##

This is free software, licensed under the Apache License, Version 2.0.
