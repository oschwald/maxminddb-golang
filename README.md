# MaxMind DB Reader for Go #

## Warning ##

This is alpha code. Use at your own risk. This is not an official MaxMind API.

## Description ##

This is a Go reader for the MaxMind DB format. This can be used to read
[GeoLite2](http://dev.maxmind.com/geoip/geoip2/geolite2/) and
[GeoIP2](http://www.maxmind.com/en/geolocation_landing) databases.

## Status ##

This API is functional, but still needs quite a bit of work to be ready for
production use. Here are some things that need to be done:

* Currently this API provides a `Lookup` method that just returns an
  `interface{}`. In the future, there will be functionality to deserialize
  the data to a specified struct value, similar to the decoding in
  `encoding/json`.
* Add support for uint128.
* There are currently only tests for the decoder code, not for the reader
  code.
* The metadata needs to be put into a struct. The current type assertions
  are gross.
* Docs need to be written.
* The code should be made idiomatic.
* Verify that arrays/slices are being passed around correctly.
* Although IPv4 addresses work, the code to speed up IPv4 lookups is not
  working as ParseIP always seems to return 16 bytes.
* Error handling should be improved.
* Travis needs to be set up for testing.

## Example ##

```go

package main

import (
    "log"
    "maxminddb"
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
    db.Close()
}

```

## Contributing ##

Contributions welcome! Please fork the repository and open a pull request
with your changes.

## License ##

This is free software, licensed under the Apache License, Version 2.0.
