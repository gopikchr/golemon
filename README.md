# Go lemon port

A port of the [lemon parser](https://www.sqlite.org/lemon.html) to Go.

## State

This work was done entirely in support of
[gopikchr](https://github.com/gopikchr/gopikchr). It works well enough
to turn `pikchr.y` into `pikchr.go`, but there are almost certainly
further bugs. Pull requests are welcome: I intend no support for this
project, but if you're obscure enough to want a Go port of the Lemon
Parser, then we share a strange kind of kinship, and I welcome your
contributions.

## Goals

- Keep the code structure as close to the original as possible, to make tracking
  and applying future changes as straight-forward as possible.
- Convert to Go idioms only where the conversion remains clear.

## See also

* [nsf/golemon](https://github.com/nsf/golemon) - 11 years out-of date 😞. Went
  in the opposite direction: the parser generator is still written in C, but
  generates Go code

## Changes

- You must define `func testcase(bool)` in your code.
- The various `#define`s have been turned into constants.

## TODOs

- [ ] Use the [embed](https://pkg.go.dev/embed) package to embed the template in the binary.
- [ ] Create a github action that follows the rss feed for changes to
      `lemon.c` and `lempar.c` and and creates issues.
- [ ] Figure out a better way to do constants: either put them in a
      separate file that is only (re)generated optionally, or make
      them settable with flags, or something.

## Contributors

- [@zellyn](https://github.com/zellyn)
