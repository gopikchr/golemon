# Notes

## Changes

### commit 4fa5952090

    Enhance Lemon so that it remembers which -D command-line options are actually
    used in the grammar and includes a list of all such options in the header
    of the generated output file.

    FossilOrigin-Name: c47a4dbd24b8277c57b7a83a8c0aeac2bc8f6ab75d1b65ba5e1fa83d1868d95f

* Turned argv0 into arvc and argv, for consistency
* Ignored the stuff that stashed and reports `-D` options since our CLI is
  quite different and most of its variations don't make any sense in Go.

### commit 82bf13796a

    Experimental changes that prevent parser stack overflows by growing the
    parser stack with heap memory when it reaches its limit.

    FossilOrigin-Name: 3fd062905fc20507b7cfc97fa976ac5b57c5b68926bf9136bd5ea4265d2d6528

* Made sure to create `YYDYNSTACK` and `YYGROWABLESTACK` in case they are used (unlikely)
* Didn't implement `realloc` and `free` because they don't really make sense in Go

### commit 51f652de10

    Bug fixes in the function that expands the parser stack.

    FossilOrigin-Name: e91179fe849760771c3508b1e7d75325183e5c3b029752d0a97dbdbd57188b97

* Not relevant to Go version.

### commit 7659ce22c5

    Optimizations to ParseFinalize() to make up for the extra cleanup associated
    with the allocated parser stack.  This branch now runs faster than trunk
    and is less than 300 bytes larger.

    FossilOrigin-Name: f7290db63cc2568090c14dffc4ea4eadfacb5b94b50a1852ef6eefd9e2e32533

* Converted updated code.

### commit 21bdfe5884

    Performance enhancements to the parser template.

    FossilOrigin-Name: 2db8b30acdeaeaf7ec92dc0382a25f96bca4561fb68a72713ff963e27f39c63b

* Made equivalent changes
* Ignored stack-resizing changes as irrelevant to the Go version

### commit 3ab9c021ff

    Fix harmless compiler warnings seen with MSVC.

    FossilOrigin-Name: e52c87420b072fa68d921eda66069542d50accbfaf1110ac4cc1543a4162200d

* Changes are irrelevant to Go version

### commit 825596481f

    Fix (totally harmless) memory leaks in Lemon to avoid warnings during ASAN
    builds.

    FossilOrigin-Name: ce009205a8edc02b7d45ac01bd0e692c3d2c3ffeadb68e4f1bad20c39075e692

* This surprisingly didn't affect the Go version: it was all about
  malloc, realloc, and free.

### commit 199f091a95

    Enhance lemon.c so that when it shows the compile-time options in the header
    comment of the generated output file, it shows all options, even those not
    used, and it shows them in sorted order.

    FossilOrigin-Name: eed76e6698eabe47c6bf9696599ce1c2f7aa428cf60f39d0566fbd0d1f6c4c62

* TODO

### commit 8cf7bd5448

    In lemon, show all the -D options in the generated header, even if none of them
    are used.

    FossilOrigin-Name: 2aa009c38bb207ac59b9bbd6f8e0d7315697b3fd6a01f9431f29a3c7ccad53e7

* TODO

### commit 51b3b402c4

    Revert Lemon so that it only shows -D options that are actually used.  Though
    the change to display the options in sorted order is retained.

    FossilOrigin-Name: e54eb217c9508c19aee085b111a1323c9009f014ba4db6019918e27002c4ca8c

* TODO

### commit 543ee479eb

    Enhance lemon so that it accepts the -U command-line option that undefines
    a preprocessor macro.

    FossilOrigin-Name: e2188a3edf3576963b45e9ffe6ef53e2a85aa68ea3dfb3243b4943d06ffaf829
