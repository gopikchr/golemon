# Intermediate stage of port from lemon.c to lemongo.go

`lemonc.go` is a direct Go port of `lemon.c`: it is implemented in Go,
but otherwise functions like `lemon.c`: it expects a `lempar.c`
template, and generates C code.

It is included here for curiosity's sake, because I ported it as an
intermediate step, and in case it proves useful for porting future
upstream updates.
