# MaxMind DB Reader for Go #

[![Build Status](https://travis-ci.org/oschwald/maxminddb-golang.png?branch=master)](https://travis-ci.org/oschwald/maxminddb-golang)

This is a Go reader for the MaxMind DB format. This can be used to read
[GeoLite2](http://dev.maxmind.com/geoip/geoip2/geolite2/) and
[GeoIP2](http://www.maxmind.com/en/geolocation_landing) databases.

This is not an official MaxMind API.

## Status ##

This API should be functional, particularly when used with the
[geoip2 API](https://github.com/oschwald/geoip2-golang). That said, the
following work remains to be done:

* Docs need to be written.
* The code should be made idiomatic.
* The error handling, particularly related to reflection, should be improved.
* The speed of the API could be improved. On my computer, I get about 20,000
  lookups per second with this API as compared to 50,000 lookups per second
  with the Java API.

Pull requests and patches are encouraged.

## Example Decoding to a Struct ##

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
    err := db.Lookup(ip, &record)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(record)
    db.Close()
}

```

## Example Decoding to an Interface ##

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

    var record interface{}
    err := db.Lookup(ip, &record)
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
