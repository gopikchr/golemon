# Quick Reference: C-to-Go Conversion Patterns

This is a cheat sheet for the most common conversion patterns found in the golemon.go port.

## Type Conversions

### Enums
```c
// C
enum symbol_type { TERMINAL, NONTERMINAL, MULTITERMINAL };
enum e_assoc { LEFT, RIGHT, NONE, UNK };
```

```go
// Go
type symbol_type = int
const (
    TERMINAL symbol_type = iota
    NONTERMINAL
    MULTITERMINAL
)

type e_assoc int
const (
    LEFT e_assoc = iota
    RIGHT
    NONE
    UNK
)
```

### Structs with Pointers
```c
// C
typedef struct config {
    struct rule *rp;
    int dot;
    struct config *next;
} config;
```

```go
// Go
type config struct {
    rp   *rule
    dot  int
    next *config
}
```

## Memory & Allocation

### Allocation
```c
// C
struct symbol *sp = malloc(sizeof(struct symbol));
memset(sp, 0, sizeof(*sp));
```

```go
// Go
sp := &symbol{}  // Zero-initialized automatically
```

### Pointer Arrays
```c
// C
struct symbol **symbols;
int nsymbol;
symbols = malloc(nsymbol * sizeof(struct symbol*));
symbols[i] = Symbol_new();
```

```go
// Go
symbols := []*symbol{}
symbols = append(symbols, Symbol_new())
// No need for nsymbol; use len(symbols)
```

### Freelist Pattern
```c
// C
static struct plink *plink_freelist = 0;

static struct plink *Plink_new(void) {
    struct plink *newlink;
    if(plink_freelist == 0) {
        plink_freelist = malloc(sizeof(struct plink)*100);
        /* link together */
    }
    newlink = plink_freelist;
    plink_freelist = plink_freelist->next;
    return newlink;
}
```

```go
// Go
var plink_freelist *plink

func Plink_new() *plink {
    var newlink *plink
    if plink_freelist == nil {
        amt := 100
        temp := make([]plink, amt)
        plink_freelist = &temp[0]
        for i := 0; i < amt-1; i++ {
            temp[i].next = &temp[i+1]
        }
        temp[amt-1].next = nil
    }
    newlink = plink_freelist
    plink_freelist = plink_freelist.next
    return newlink
}
```

## Strings

### Basic Usage
```c
// C
char *name;
name = malloc(strlen(x) + 1);
strcpy(name, x);
if(strcmp(a, b) == 0) { /* ... */ }
```

```go
// Go
name := x  // Direct assignment, no allocation
if a == b { /* ... */ }
```

### Character Loops
```c
// C
const char *z = "hello";
for(i = 0; z[i] != 0; i++) {
    if(ISLOWER(z[i])) { /* ... */ }
}
```

```go
// Go
z := "hello"
runes := []rune(z)
for i, c := range runes {
    if unicode.IsLower(c) { /* ... */ }
}
```

### String Functions
```c
// C
char buf[256];
lemon_sprintf(buf, "%d %s", val, str);
fprintf(fp, "Output: %s\n", buf);
```

```go
// Go
buf := fmt.Sprintf("%d %s", val, str)
fmt.Fprintf(fp, "Output: %s\n", buf)
```

## Linked List Iteration

### Loop Pattern
```c
// C
for(rp = head; rp != NULL; rp = rp->next) {
    int i;
    for(i = 0; i < rp->nrhs; i++) {
        /* process rp->rhs[i] */
    }
}
```

```go
// Go
for rp := head; rp != nil; rp = rp.next {
    for i := range rp.rhs {
        /* process rp.rhs[i] */
    }
}
```

### Pointer Dereferencing
```c
// C
struct symbol *sp = rp->lhs;
sp->index = 5;
rp->rhs[i]->name;
```

```go
// Go
sp := rp.lhs  // No -> operator needed
sp.index = 5
rp.rhs[i].name
```

## Macros

### Value Macros
```c
// C
#define MAXRHS 1000
#define NO_OFFSET (-2147483647)
#define NDEBUG
```

```go
// Go
const MAXRHS = 1000
const NO_OFFSET = -2147483647
const NDEBUG = true
```

### Function-like Macros
```c
// C
#define SetFind(X,Y) (X[Y])
#define ISLOWER(X) islower((unsigned char)(X))
#define acttab_lookahead_size(X) ((X)->nAction)
```

```go
// Go
func SetFind(s map[int]bool, e int) bool {
    return s[e]
}

func islower(r rune) bool {
    return unicode.IsLetter(r) && !unicode.IsUpper(r)
}

func acttab_lookahead_size(x *acttab) int {
    return x.nAction
}
```

### Conditional Code
```c
// C
#ifdef DEBUG
printf("Debug: %d\n", x);
#endif

#if 0
/* Old code to keep around */
fprintf(out,"#if INTERFACE\n");
#endif
```

```go
// Go
if false {  // DEBUG = false
    fmt.Printf("Debug: %d\n", x)
}

if false {  // #if 0
    fmt.Printf("#if INTERFACE\n")
}
```

## Control Flow

### For Loops
```c
// C
for(i = 0; i < 10; i++) { /* ... */ }
for(;;) { if(done) break; }
```

```go
// Go
for i := 0; i < 10; i++ { /* ... */ }
for { if done { break } }
```

### While Loops
```c
// C
while(p->nLookahead > 0) {
    process();
}
```

```go
// Go
for p.nLookahead > 0 {
    process()
}
```

### Switch Statements
```c
// C
switch(typ) {
case SHIFT:
    fprintf(fp, "shift");
    break;
case REDUCE:
    fprintf(fp, "reduce");
    break;
default:
    result = false;
    break;
}
```

```go
// Go
switch typ {
case SHIFT:
    fmt.Fprintf(fp, "shift")
case REDUCE:
    fmt.Fprintf(fp, "reduce")
default:
    result = false
}
```

## Functions

### Function Definitions
```c
// C
static void FindRulePrecedences(struct lemon *xp) {
    int i;
    struct rule *rp;
    for(rp = xp->rule; rp != NULL; rp = rp->next) {
        /* ... */
    }
}
```

```go
// Go
func FindRulePrecedences(xp *lemon) {
    var i int
    var rp *rule
    for rp = xp.rule; rp != nil; rp = rp.next {
        /* ... */
    }
}
```

### Return Multiple Values
```c
// C
int compute_result(struct state *stp, int *result_ptr) {
    *result_ptr = compute_value();
    return 0;
}
```

```go
// Go
func computeResult(stp *state) (int, int) {
    result := computeValue()
    return result, 0
}
```

### Variadic Arguments
```c
// C
void ErrorMsg(const char *filename, int lineno, const char *format, ...) {
    va_list ap;
    va_start(ap, format);
    vfprintf(stderr, format, ap);
    va_end(ap);
}
```

```go
// Go
func ErrorMsg(filename string, lineno int, format string, args ...interface{}) {
    fmt.Fprintf(os.Stderr, "%s:%d: ", filename, lineno)
    fmt.Fprintf(os.Stderr, format, args...)
    fmt.Fprintf(os.Stderr, "\n")
}
```

## File I/O

### File Opening
```c
// C
FILE *fp = fopen(filename, "wb");
if(fp == NULL) {
    fprintf(stderr, "Can't open\n");
    return;
}
/* ... use fp ... */
fclose(fp);
```

```go
// Go
fp, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
if err != nil {
    fmt.Fprintf(os.Stderr, "Can't open\n")
    return
}
defer fp.Close()
/* ... use fp ... */
```

### File Reading
```c
// C
FILE *fp = fopen(filename, "rb");
char buf[1000];
fread(buf, sizeof(char), 1000, fp);
```

```go
// Go
bytes, err := os.ReadFile(filename)
if err != nil { /* handle */ }
filebuf := []rune(string(bytes))
```

## Global Variables

### Simple Globals
```c
// C
static int actionIndex = 0;
static struct config *current = 0;
static int showPrecedenceConflict = 0;
```

```go
// Go
var actionIndex = 0
var current *config
var showPrecedenceConflict bool
```

### Global Counters
```c
// C
static int actionIndex = 0;

struct action *Action_new(void) {
    struct action *a = malloc(sizeof(*a));
    actionIndex++;
    a->index = actionIndex;
    return a;
}
```

```go
// Go
var actionIndex = 0

func Action_new() *action {
    actionIndex++
    return &action{
        index: actionIndex,
    }
}
```

## Sorting

### Comparison Function
```c
// C
int Symbolcmpp(const void *a, const void *b) {
    struct symbol *sa = *(struct symbol**)a;
    struct symbol *sb = *(struct symbol**)b;
    return strcmp(sa->name, sb->name);
}
```

```go
// Go
func Symbolcmpp(a, b *symbol) int {
    if a.name < b.name { return -1 }
    if a.name > b.name { return 1 }
    return 0
}
```

### Using sort.Interface
```c
// C
qsort(lemp->symbols, lemp->nsymbol, sizeof(lemp->symbols[0]), Symbolcmpp);
```

```go
// Go
type symbolSorter []*symbol

func (s symbolSorter) Len() int { return len(s) }
func (s symbolSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s symbolSorter) Less(i, j int) bool { return Symbolcmpp(s[i], s[j]) < 0 }

sort.Sort(symbolSorter(lemp.symbols))
```

## Maps vs Arrays

### Sets
```c
// C
#define SetFind(X,Y) (X[Y])
int *firstset;
SetAdd(firstset, 5);
if(SetFind(firstset, 5)) { /* ... */ }
```

```go
// Go
firstset := make(map[int]bool)
firstset[5] = true
if firstset[5] { /* ... */ }
```

### Symbol Tables
```c
// C
struct symbol *Symbol_find(const char *name) {
    for(sp = first; sp; sp = sp->next) {
        if(strcmp(sp->name, name) == 0) return sp;
    }
    return 0;
}
```

```go
// Go
var symbolTable = make(map[string]*symbol)

func Symbol_find(name string) *symbol {
    return symbolTable[name]
}
```

## Common Patterns Summary

| C Pattern | Go Pattern | Notes |
|-----------|-----------|-------|
| `NULL` | `nil` | Pointer null value |
| `malloc(sizeof(x))` | `&x{}` | Allocation with zero init |
| `char*` | `string` | Immutable strings |
| `char[]` + index | `[]rune` | Character sequences |
| `struct x *p` | `p *x` | Pointers in declarations |
| `p->field` | `p.field` | Implicit dereferencing |
| `array[i]` | `slice[i]` | Indexing (same syntax) |
| `for(;;)` | `for { }` | Infinite loop |
| `while(c)` | `for c { }` | Conditional loop |
| `fopen()` | `os.OpenFile()` | File opening |
| `fprintf()` | `fmt.Fprintf()` | Formatted output |
| `#define X Y` | `const X = Y` | Value constants |
| `typedef struct x { }` | `type x struct { }` | Type definition |

---

For detailed explanations and more examples, see `C_TO_GO_PORTING_GUIDE.md`
