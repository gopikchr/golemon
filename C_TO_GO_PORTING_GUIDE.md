# C to Go Porting Guide for Lemon Parser

## Comprehensive Conversion Patterns from lemon.c/lempar.c to Go

This document details the mechanical patterns used to port the LEMON parser generator from C to Go, based on analysis of golemon.go, lemonc.go, and lempar.go.tpl.

---

## 1. TYPE CONVERSIONS

### 1.1 Primitive Type Mappings

| C Type | Go Type | Context | Notes |
|--------|---------|---------|-------|
| `int` | `int` | General integers | Direct mapping |
| `unsigned char` | `byte` or `rune` | Characters/bytes | Use `rune` for Unicode support |
| `char*` | `string` | Strings | Always use Go's immutable strings |
| `char` (single) | `rune` | Single characters | For Unicode character operations |
| `void*` | Interface/pointer | Generic pointers | Use specific types when possible |
| `size_t` | `int` | Sizes | Go conventions use `int` |
| `FILE*` | `*os.File` | File handles | Use Go's standard library |

### 1.2 Type Aliases

**Pattern**: Use `type` keyword to create type aliases matching C enum-like definitions.

**C Code Example**:
```c
enum symbol_type { TERMINAL, NONTERMINAL, MULTITERMINAL };
```

**Go Equivalent**:
```go
type symbol_type = int

const (
    TERMINAL symbol_type = iota
    NONTERMINAL
    MULTITERMINAL
)
```

**Pattern Explanation**:
- Use `type symbol_type = int` to define a named type
- Use `const (` blocks with `iota` for sequential enumeration values
- First item in `const (` starts at 0 by default

### 1.3 Struct Type Conversions

**Pattern**: C structs convert directly to Go structs with field name changes.

**C Code Example**:
```c
typedef struct symbol {
  char *name;           /* Name of the symbol */
  int index;            /* Index number for this symbol */
  enum symbol_type typ; /* Symbols are all either TERMINALS or NTs */
  struct rule *rule;    /* Linked list of rules of this (if an NT) */
} symbol;
```

**Go Equivalent**:
```go
type symbol struct {
    name   string       /* Name of the symbol */
    index  int          /* Index number for this symbol */
    typ    symbol_type  /* Symbols are all either TERMINALS or NTs */
    rule   *rule        /* Linked list of rules of this (if an NT) */
}
```

**Naming Convention**: Field names keep their C names but are lowercase (Go convention).

### 1.4 Union Types

**Pattern**: C unions are converted to struct with pointer to one-of-many pattern.

**C Code Example**:
```c
union {
    struct state *stp;  /* The new state, if a shift */
    struct rule *rp;    /* The rule, if a reduce */
}
```

**Go Equivalent**:
```go
type stateOrRuleUnion struct {
    stp *state /* The new state, if a shift */
    rp  *rule  /* The rule, if a reduce */
}
```

Then store this in an `action` struct and check using a `typ` discriminator field:
```go
type action struct {
    typ e_action
    x   stateOrRuleUnion
}
```

Access pattern:
```c
// C: cast based on type
action->x.stp  // if type == SHIFT
action->x.rp   // if type == REDUCE
```

```go
// Go: same pattern
switch ap.typ {
case SHIFT:
    stp := ap.x.stp
case REDUCE:
    rp := ap.x.rp
}
```

---

## 2. ARRAYS VS SLICES

### 2.1 Fixed-Size Arrays

**Pattern**: C fixed arrays become Go slices for dynamic structures; only use arrays for constants.

**C Code Example**:
```c
char zTemp[50];  /* Fixed buffer */
struct config *cfp;
cfp->fws = x[Y];  /* Array subscript access */
```

**Go Equivalent**:
```go
var zTemp [50]rune  /* Fixed array for constants only */
// OR
zTemp := make([]rune, 50)  /* Slice for resizable */

cfp.fws = m[y]  /* Map access or slice index */
```

### 2.2 Dynamic Arrays/Linked Lists

**Pattern**: C linked lists (via pointers) stay as pointers; C arrays become slices.

**C Code Example - Linked List**:
```c
struct config *cfp;
for(cfp = current; cfp != NULL; cfp = cfp->next) {
    /* Process cfp */
}
```

**Go Equivalent**:
```go
var cfp *config
for cfp := current; cfp != nil; cfp = cfp.next {
    /* Process cfp */
}
```

**C Code Example - Array**:
```c
/* In struct */
struct symbol **subsym;  /* Array of constituent symbols */
int nsubsym;             /* Count */
for(i = 0; i < nsubsym; i++) {
    sp = subsym[i];
}
```

**Go Equivalent**:
```go
/* In struct */
subsym []*symbol  /* Slice of constituent symbols */
// nsubsym is NOT needed - use len(subsym) instead

for i := range subsym {
    sp := subsym[i]
}
```

### 2.3 Array Initialization and Growth

**Pattern**: C array growth via malloc/realloc becomes Go slice append pattern.

**C Code Example**:
```c
if( p->nLookahead >= p->nLookaheadAlloc ){
    p->nLookaheadAlloc += 25;
    p->aLookahead = (struct lookahead_action *)
        realloc( p->aLookahead, p->nLookaheadAlloc * sizeof(p->aLookahead[0]) );
}
```

**Go Equivalent**:
```go
if p.nLookahead >= p.nLookaheadAlloc {
    p.nLookaheadAlloc += 25
    p.aLookahead = append(p.aLookahead, make([]lookahead_action, 25)...)
}
```

### 2.4 Set Data Structure

**Pattern**: C bit-sets or arrays become Go maps for sparse data.

**C Code Example**:
```c
#define SetAdd(X,Y) (X[Y]=1)
#define SetFind(X,Y) (X[Y])
struct symbol {
    int *firstset;  /* First-set for all rules of this symbol */
};
```

**Go Equivalent**:
```go
type symbol struct {
    firstset map[int]bool  /* First-set for all rules of this symbol */
}

func SetFind(s map[int]bool, e int) bool {
    return s[e]
}

func SetAdd(s map[int]bool, e int) bool {
    old := s[e]
    s[e] = true
    return s[e] != old  /* Return true if changed */
}
```

---

## 3. STRING AND CHARACTER HANDLING

### 3.1 String Types

**Pattern**: All C `char*` becomes Go `string`.

**C Code Example**:
```c
char *name;
char *code;
name = malloc(strlen(x) + 1);
strcpy(name, x);
```

**Go Equivalent**:
```go
name string
code string
name = x  /* Direct assignment, no allocation needed */
```

### 3.2 Character Access

**Pattern**: Convert `char*` to `[]rune` for character-by-character processing.

**C Code Example**:
```c
const char *z = "hello";
for(i = 0; z[i] != 0; i++) {
    if(ISLOWER(z[i])) { /* Process */ }
}
```

**Go Equivalent**:
```go
z := "hello"
runes := []rune(z)
for i, c := range runes {
    if unicode.IsLower(c) { /* Process */ }
}
```

### 3.3 String Formatting

**Pattern**: C sprintf becomes Go fmt.Sprintf; C printf becomes fmt.Printf/fmt.Fprintf.

**C Code Example**:
```c
char buf[MAXLEN];
lemon_sprintf(buf, "%d %s", value, str);
fprintf(fp, "Output: %s\n", buf);
```

**Go Equivalent**:
```go
buf := fmt.Sprintf("%d %s", value, str)
fmt.Fprintf(fp, "Output: %s\n", buf)
```

### 3.4 String Functions

| C Function | Go Equivalent | Notes |
|------------|--------------|-------|
| `strlen(s)` | `len(s)` | For Go strings |
| `strcpy(d, s)` | `d = s` | Assignment is safe in Go |
| `strcat(d, s)` | `d += s` | String concatenation |
| `strcmp(a, b)` | `a == b` | Direct comparison for strings |
| `strchr(s, c)` | `strings.ContainsRune(s, c)` | Or use strings.Index |
| `memcpy(d, s, n)` | `copy(d, s)` | For slices |
| `memset(p, v, n)` | Use range loop or fill slice | `for i := range sl { sl[i] = v }` |

### 3.5 Character Classification

**Pattern**: C `<ctype.h>` macros become Go `unicode` package functions.

**C Code Example**:
```c
#define ISSPACE(X) isspace((unsigned char)(X))
#define ISLOWER(X) islower((unsigned char)(X))
#define ISUPPER(X) isupper((unsigned char)(X))
#define ISALPHA(X) isalpha((unsigned char)(X))
#define ISDIGIT(X) isdigit((unsigned char)(X))
#define ISALNUM(X) isalnum((unsigned char)(X))

if(ISLOWER(c)) { /* ... */ }
```

**Go Equivalent**:
```go
func islower(r rune) bool {
    return unicode.IsLetter(r) && !unicode.IsUpper(r)
}

if islower(c) { /* ... */ }

// Or use directly from unicode package:
if unicode.IsLetter(c) { /* ... */ }
if unicode.IsSpace(c) { /* ... */ }
if unicode.IsUpper(c) { /* ... */ }
if unicode.IsDigit(c) { /* ... */ }
```

### 3.6 String Splitting and Indexing

**Pattern**: String operations use strings package.

**C Code Example**:
```c
char *path = "/some/file.txt";
char *lastSlash = strrchr(path, '/');
char *lastDot = strrchr(path, '.');
```

**Go Equivalent**:
```go
path := "/some/file.txt"
lastSlash := strings.LastIndex(path, "/")  // returns -1 if not found
lastDot := strings.LastIndex(path, ".")
if lastSlash != -1 {
    path = path[lastSlash:]
}
```

---

## 4. MEMORY MANAGEMENT

### 4.1 Dynamic Allocation

**Pattern**: C malloc becomes Go `make()` or `&struct{}` for direct allocation.

**C Code Example**:
```c
struct symbol *sp = malloc(sizeof(struct symbol));
memset(sp, 0, sizeof(*sp));
sp->name = malloc(strlen(name) + 1);
strcpy(sp->name, name);
```

**Go Equivalent**:
```go
sp := &symbol{}  /* Allocated and zeroed automatically */
sp.name = name   /* No separate allocation needed */
```

### 4.2 Pointers

**Pattern**: C pointers map directly to Go pointers.

**C Code Example**:
```c
struct symbol **symbols;  /* Array of pointers */
symbols = malloc(nsymbol * sizeof(struct symbol*));
symbols[i] = Symbol_new();
```

**Go Equivalent**:
```go
symbols []*symbol  /* Slice of pointers */
symbols = append(symbols, Symbol_new())
```

### 4.3 NULL Pointer Checks

**Pattern**: C NULL becomes Go nil.

**C Code Example**:
```c
if(cfp == NULL) { return; }
if(cfp != NULL) { /* ... */ }
```

**Go Equivalent**:
```go
if cfp == nil { return }
if cfp != nil { /* ... */ }
```

### 4.4 Pointer Dereferences

**Pattern**: `->` becomes `.` for pointer member access (Go does implicit dereferencing).

**C Code Example**:
```c
action->sp->name
config->rp->nrhs
state->ap->next
```

**Go Equivalent**:
```go
action.sp.name  /* No -> needed */
config.rp.lhs
state.ap.next
```

### 4.5 Object Pools/Freelists

**Pattern**: C memory pools using linked lists stay the same structure.

**C Code Example**:
```c
static struct plink *plink_freelist = 0;

static struct plink *Plink_new(void) {
    struct plink *newlink;
    if( plink_freelist==0 ){
        plink_freelist = (struct plink*)malloc(sizeof(struct plink)*100);
        /* Link them together */
    }
    newlink = plink_freelist;
    plink_freelist = plink_freelist->next;
    return newlink;
}
```

**Go Equivalent**:
```go
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

---

## 5. MACROS/DEFINES CONVERSION

### 5.1 Simple Value Macros

**Pattern**: `#define NAME value` becomes Go `const NAME = value`.

**C Code Example**:
```c
#define MAXRHS 1000
#define NO_OFFSET (-2147483647)
```

**Go Equivalent**:
```go
const MAXRHS = 1000
const NO_OFFSET = -2147483647
```

### 5.2 Function-like Macros

**Pattern**: `#define MACRO(args)` becomes Go helper function.

**C Code Example**:
```c
#define SetFind(X,Y) (X[Y])
#define ISSPACE(X) isspace((unsigned char)(X))
#define ISALPHA(X) isalpha((unsigned char)(X))
#define acttab_lookahead_size(X) ((X)->nAction)
```

**Go Equivalent**:
```go
func SetFind(s map[int]bool, e int) bool {
    return s[e]
}

func isalpha(r rune) bool {
    return unicode.IsLetter(r)
}

func (x *acttab) lookahead_size() int {
    return x.nAction
}
// OR for non-methods:
func acttab_lookahead_size(x *acttab) int {
    return x.nAction
}
```

### 5.3 Conditional Compilation Macros

**Pattern**: `#ifdef/#if/#ifndef` become Go conditional logic or are completely omitted.

**C Code Example**:
```c
#ifdef TEST
#define MAXRHS 5
#else
#define MAXRHS 1000
#endif

#ifdef __WIN32__
extern int access(const char *path, int mode);
#else
#include <unistd.h>
#endif

#if 0
/* Debug code */
fprintf(out,"#if INTERFACE\n");
#endif
```

**Go Equivalent**:
```go
// var MAXRHS = 5  /* For testing */
var MAXRHS = 1000

// No need for platform-specific code - Go is cross-platform
// Just use os.* directly

// For #if 0 blocks, use:
if false {
    fmt.Printf("#if INTERFACE\n")
}
```

### 5.4 Preprocessor Text Replacement

**Pattern**: Grammar files with `#define` are handled by a custom text replacement engine.

**Implementation Pattern**:
```go
type defines struct {
    mappings map[string]string
    re       *regexp.Regexp
}

func (d *defines) addDefine(define, replacement string) {
    if d.mappings == nil {
        d.mappings = make(map[string]string)
    }
    d.mappings[define] = replacement
    d.re = nil  /* Rebuild regexp on next use */
}

func (d defines) replaceAll(s string) string {
    if len(d.mappings) == 0 {
        return s
    }
    if d.re == nil {
        d.buildRegexp()
    }
    lines := strings.Split(s, "\n")
    for i, line := range lines {
        if strings.HasPrefix(line, "**    ") {
            continue
        }
        lines[i] = d.re.ReplaceAllStringFunc(line, d.replaceFunc)
    }
    return strings.Join(lines, "\n")
}

func (d defines) replaceFunc(match string) string {
    return d.mappings[match]
}

func (d *defines) buildRegexp() {
    keys := make([]string, 0, len(d.mappings))
    for key := range d.mappings {
        keys = append(keys, key)
    }
    sort.Strings(keys)
    d.re = regexp.MustCompile(`\b(` + strings.Join(keys, "|") + `)\b`)
}
```

---

## 6. CLI FLAGS AND ARGUMENT PROCESSING

### 6.1 Command-Line Arguments

**Pattern**: C `argc`/`argv` become Go `os.Args` and `flag` package.

**C Code Example**:
```c
int main(int argc, char **argv) {
    /* Process argv */
    for(i = 1; i < argc; i++) {
        if(argv[i][0] == '-') {
            /* Option */
        } else {
            /* Filename */
        }
    }
}
```

**Go Equivalent**:
```go
func main() {
    flag.BoolVar(&compress, "c", false, "Don't compress the action table.")
    flag.StringVar(&outputDir, "d", "", "Output directory. Default '.'")
    flag.Parse()
    
    args := flag.Args()  /* Non-flag arguments */
    if len(args) != 1 {
        /* Error */
    }
    filename := args[0]
}
```

### 6.2 Custom Flag Types

**Pattern**: Flags that can appear multiple times become custom `flag.Value` implementations.

**C Code Example**:
```c
/* -D NAME=VALUE and -U NAME options */
struct s_options options[] = {
    {OPT_STR, "D", &azDefine, "Define an %ifdef macro"},
    {OPT_STR, "U", &azUndefine, "Undefine a macro"},
};
```

**Go Equivalent**:
```go
type setFlag struct {
    values map[string]string
}

func (s setFlag) String() string {
    /* ... */
}

func (s setFlag) Set(value string) error {
    /* Handle -D key=value or -D key */
    parts := strings.SplitN(value, "=", 2)
    if len(parts) == 2 {
        s.values[parts[0]] = parts[1]
    } else {
        s.values[parts[0]] = "1"
    }
    return nil
}

var azDefine setFlag
flag.Var(&azDefine, "D", "Define an %ifdef macro.")
flag.Var(&azDefine, "U", "Undefine a macro.")
```

### 6.3 Global State for Arguments

**Pattern**: Command-line state stored in `lemon` struct instead of global variables.

```go
type lemon struct {
    argc   int       /* Number of command-line arguments */
    argv   []string  /* Command-line arguments */
    /* ... other fields ... */
}

func main() {
    lem.argc = len(os.Args)
    lem.argv = os.Args
    lem.filename = flag.Args()[0]
}
```

---

## 7. NAMING CONVENTIONS

### 7.1 Functions

**Pattern**: Function names stay exactly as in C, no camelCase conversion.

**C Code**:
```c
void FindRulePrecedences(struct lemon*);
void FindFirstSets(struct lemon*);
struct symbol *Symbol_new(char *name);
```

**Go Equivalent**:
```go
func FindRulePrecedences(xp *lemon)
func FindFirstSets(lemp *lemon)
func Symbol_new(name string) *symbol
```

Exported functions (public) start with uppercase; internal functions start with lowercase.

### 7.2 Type Names

**Pattern**: Struct names stay the same but use Go naming (lowercase for internal, mixed for exported).

**C Code**:
```c
typedef struct symbol { ... } symbol;
typedef struct rule { ... } rule;
```

**Go Equivalent**:
```go
type symbol struct { ... }
type rule struct { ... }
```

### 7.3 Field Names

**Pattern**: Struct field names stay the same in C format (lowerCamelCase or snake_case).

**C Code**:
```c
struct symbol {
    char *name;
    int index;
    enum symbol_type typ;
    int nsubsym;
    struct symbol **subsym;
};
```

**Go Equivalent**:
```go
type symbol struct {
    name      string
    index     int
    typ       symbol_type
    subsym    []*symbol  /* No nsubsym - use len(subsym) */
}
```

### 7.4 Constants

**Pattern**: Constants stay exactly as in C.

**C Code**:
```c
#define TERMINAL 0
#define NONTERMINAL 1
```

**Go Equivalent**:
```go
const TERMINAL = 0
const NONTERMINAL = 1
```

---

## 8. CONTROL FLOW

### 8.1 If-Else Statements

**Pattern**: Direct 1:1 mapping.

**C Code**:
```c
if(a > b) {
    x = 1;
} else if(a == b) {
    x = 0;
} else {
    x = -1;
}
```

**Go Equivalent**:
```go
if a > b {
    x = 1
} else if a == b {
    x = 0
} else {
    x = -1
}
```

### 8.2 For Loops

**Pattern**: C `for(init; cond; inc)` becomes equivalent Go `for`.

**C Code**:
```c
for(i = 0; i < 10; i++) {
    printf("%d\n", i);
}
```

**Go Equivalent**:
```go
for i := 0; i < 10; i++ {
    fmt.Printf("%d\n", i)
}
```

**Pattern**: C `for(;;)` becomes Go `for`.

**C Code**:
```c
for(;;) {
    if(done) break;
}
```

**Go Equivalent**:
```go
for {
    if done { break }
}
```

### 8.3 Linked List Iteration

**Pattern**: Pointer traversal becomes exact same pattern.

**C Code**:
```c
for(cfp = current; cfp != NULL; cfp = cfp->next) {
    process(cfp);
}
```

**Go Equivalent**:
```go
for cfp := current; cfp != nil; cfp = cfp.next {
    process(cfp)
}
```

### 8.4 While Loops

**Pattern**: `while(cond)` becomes `for cond`.

**C Code**:
```c
while(p->nLookahead > 0) {
    process();
}
```

**Go Equivalent**:
```go
for p.nLookahead > 0 {
    process()
}
```

### 8.5 Do-While Loops

**Pattern**: `do { } while(cond)` needs restructuring or a flag variable.

**C Code**:
```c
do {
    x = process();
} while(x > 0);
```

**Go Equivalent**:
```go
for {
    x := process()
    if x <= 0 { break }
}
```

### 8.6 Switch Statements

**Pattern**: Direct mapping; Go doesn't require `break`.

**C Code**:
```c
switch(typ) {
case SHIFT:
    { /* code */ }
    break;
case REDUCE:
    { /* code */ }
    break;
default:
    result = false;
    break;
}
```

**Go Equivalent**:
```go
switch typ {
case SHIFT:
    /* code */
case REDUCE:
    /* code */
default:
    result = false
}
```

### 8.7 Fall-Through Cases

**Pattern**: Use explicit `fallthrough` keyword.

**C Code**:
```c
switch(ap->type) {
case SRCONFLICT:
case RRCONFLICT:
    fprintf(fp, "conflict");
    break;
}
```

**Go Equivalent**:
```go
switch ap.typ {
case SRCONFLICT:
    fallthrough
case RRCONFLICT:
    fmt.Fprintf(fp, "conflict")
}
```

### 8.8 Break and Continue

**Pattern**: Exact same in Go.

**C Code**:
```c
for(i = 0; i < n; i++) {
    if(skip[i]) continue;
    if(done) break;
    process(i);
}
```

**Go Equivalent**:
```go
for i := 0; i < n; i++ {
    if skip[i] { continue }
    if done { break }
    process(i)
}
```

---

## 9. FUNCTIONS

### 9.1 Function Definitions

**Pattern**: Direct mapping from C to Go.

**C Code**:
```c
static void FindRulePrecedences(struct lemon *xp)
{
    int i;
    struct rule *rp;
    for(rp = xp->rule; rp != NULL; rp = rp->next) {
        /* ... */
    }
}
```

**Go Equivalent**:
```go
func FindRulePrecedences(xp *lemon) {
    var i int
    var rp *rule
    for rp = xp.rule; rp != nil; rp = rp.next {
        /* ... */
    }
}
```

### 9.2 Function Return Values

**Pattern**: Go allows multiple return values directly.

**C Code**:
```c
int function_returning_status(struct state *stp, int *result_ptr) {
    *result_ptr = compute_value();
    return 0;  /* status */
}
```

**Go Equivalent**:
```go
func functionReturningStatus(stp *state) (int, int) {
    result := computeValue()
    return result, 0  /* result, status */
}

// Call:
result, status := functionReturningStatus(stp)
```

### 9.3 Function Pointers

**Pattern**: Go function types become explicit type definitions.

**C Code**:
```c
int (*compare_func)(const char*, const char*);
compare_func = strcmp;
```

**Go Equivalent**:
```go
type compareFunc func(string, string) int
var cmpFunc compareFunc
cmpFunc = strings.Compare  /* or custom function */
```

### 9.4 Variable Arguments (varargs)

**Pattern**: `...` for variadic arguments.

**C Code**:
```c
void ErrorMsg(const char *filename, int lineno, const char *format, ...) {
    va_list ap;
    va_start(ap, format);
    vfprintf(stderr, format, ap);
    va_end(ap);
}
```

**Go Equivalent**:
```go
func ErrorMsg(filename string, lineno int, format string, args ...interface{}) {
    fmt.Fprintf(os.Stderr, "%s:%d: ", filename, lineno)
    fmt.Fprintf(os.Stderr, format, args...)
    fmt.Fprintf(os.Stderr, "\n")
}
```

### 9.5 Forward Declarations

**Pattern**: Go doesn't require forward declarations; define types at package level.

**C Code**:
```c
PRIVATE void buildshifts(struct lemon *, struct state *);
```

**Go Equivalent**: Not needed in Go. Functions can call other functions defined later in the same package.

---

## 10. GLOBAL VARIABLES

### 10.1 Module-Level Globals

**Pattern**: Global variables stay at package level.

**C Code**:
```c
static struct config *current = 0;
static struct config **currentend = &current;
static struct config *basis = 0;
static struct plink *plink_freelist = 0;
static int showPrecedenceConflict = 0;
```

**Go Equivalent**:
```go
var current *config
var currentend **config = &current
var basis *config
var plink_freelist *plink
var showPrecedenceConflict bool
```

### 10.2 Initializing Global Pointers

**Pattern**: Initialization of pointer-to-pointer variables.

**C Code**:
```c
static struct config *current = 0;
static struct config **currentend = &current;

void Configlist_init() {
    current = nil;
    currentend = &current;
}
```

**Go Equivalent**:
```go
var current *config

func Configlist_init() {
    current = nil
    // Note: Can't use currentend = &current at global level
    // Must use &current where needed or pass pointer to pointer
}
```

### 10.3 Global Counters/Flags

**Pattern**: Simple globals for counters and flags.

**C Code**:
```c
static int actionIndex = 0;

struct action *Action_new() {
    struct action *a = malloc(sizeof(*a));
    actionIndex++;
    a->index = actionIndex;
    return a;
}
```

**Go Equivalent**:
```go
var actionIndex = 0

func Action_new() *action {
    actionIndex++
    return &action{
        index: actionIndex,
    }
}
```

### 10.4 Global State Struct

**Pattern**: Many globals wrapped in a main struct to avoid global state pollution.

```go
type lemon struct {
    sorted     []*state
    rule       *rule
    nstate     int
    nsymbol    int
    nterminal  int
    /* ... many more fields ... */
    argc       int
    argv       []string
}

func main() {
    var lem lemon
    Parse(&lem)
    /* Pass &lem to functions instead of using globals */
}
```

---

## 11. FILE I/O

### 11.1 File Opening

**Pattern**: C `fopen` becomes `os.OpenFile`.

**C Code**:
```c
FILE *fp = fopen(filename, "wb");
if(fp == NULL) {
    fprintf(stderr, "Can't open file\n");
    return;
}
```

**Go Equivalent**:
```go
fp, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
if err != nil {
    fmt.Fprintf(os.Stderr, "Can't open file: %v\n", err)
    return
}
defer fp.Close()
```

### 11.2 File Reading

**Pattern**: C `fread` or reading into buffer becomes Go `os.ReadFile` or `bufio.Reader`.

**C Code**:
```c
FILE *fp = fopen(filename, "rb");
char buf[BUFSIZE];
fread(buf, sizeof(char), BUFSIZE, fp);
```

**Go Equivalent**:
```go
// Read entire file:
bytes, err := os.ReadFile(filename)
if err != nil {
    // Handle error
}
filebuf := []rune(string(bytes))

// Or use Reader for streaming:
fp, _ := os.Open(filename)
reader := bufio.NewReader(fp)
```

### 11.3 File Writing

**Pattern**: C `fprintf` becomes `fmt.Fprintf`.

**C Code**:
```c
fprintf(fp, "const int var = %d;\n", value);
```

**Go Equivalent**:
```go
fmt.Fprintf(fp, "const int var = %d;\n", value)
```

### 11.4 File Closing

**Pattern**: Go uses `defer` for automatic closing.

**C Code**:
```c
fclose(fp);
```

**Go Equivalent**:
```go
defer fp.Close()
```

---

## 12. PARSER TEMPLATE CONVERSION (lempar.c to lempar.go.tpl)

### 12.1 Template Structure

**C Template Comments**:
```c
/*
** The "lemon" program inserts text at each "%%" line.
** Any "P-a-r-s-e" identifier prefix (without the interstitial "-")
** contained in this template is changed into the value of the %name directive
*/
```

**Go Template Comments**:
```go
/*
** The "lemon" program inserts text at each "%%" line.
** Any "P-a-r-s-e" identifier prefix (without the interstitial "-")
** contained in this template is changed into the value of the %name directive
*/
```

### 12.2 Generated Code Differences

**Concept**: The template generates parser code for the target language. The Go template generates Go code while the C template generates C code.

**Key Differences**:
- Token type: `type YYCODETYPE = ...` instead of `#define YYCODETYPE`
- Stack type: Go struct instead of C union
- Methods: Receiver methods on `yyParser` struct
- Error handling: Panic instead of setjmp/longjmp

### 12.3 Defines in Template

**C Pattern**:
```c
#define YYCODETYPE %s
#define YYACTIONTYPE %s
#define YYSTACKDEPTH %s
```

**Go Pattern**:
```go
const YYCODETYPE = %s
type YYCODETYPE = %s
const YYSTACKDEPTH = %s
```

---

## 13. RECURSIVE DESCENT PARSING

### 13.1 Tokenization State Machine

**Pattern**: State machine for tracking parsing state.

**C Code**:
```c
enum parse_state_type {
    INITIALIZE, WAITING_FOR_DECL_OR_RULE, IN_RHS,
    /* ... many states ... */
};

struct pstate {
    enum parse_state_type state;
    struct symbol *lhs;
    int nrhs;
    struct symbol **rhs;
    /* ... */
};
```

**Go Equivalent**:
```go
type e_state int

const (
    INITIALIZE e_state = iota
    WAITING_FOR_DECL_OR_RULE
    IN_RHS
    /* ... many states ... */
)

type pstate struct {
    state e_state
    lhs   *symbol
    nrhs  int
    rhs   []*symbol
    /* ... */
}
```

### 13.2 Token Loop

**Pattern**: Main parsing loop with character-by-character processing.

**C Code**:
```c
for(cp = 0; cp < len; ) {
    c = input[cp];
    
    if(c == '"') {
        cp++;
        while(cp < len && input[cp] != '"') { cp++; }
        parseonetoken(&ps, token_text);
    } else if(c == '{') {
        /* Handle block */
    }
}
```

**Go Equivalent**:
```go
for cp := 0; cp < len(filebuf); {
    c := filebuf[cp]
    
    if c == '"' {
        cp++
        for cp < len(filebuf) && filebuf[cp] != '"' {
            cp++
        }
        parseonetoken(&ps, filebuf[ps.tokenstart:cp])
    } else if c == '{' {
        /* Handle block */
    }
}
```

---

## 14. SORTING AND COMPARISONS

### 14.1 Merge Sort Implementation

**Pattern**: Go's `sort` package or custom merge sort.

**C Code**:
```c
static struct action *msort(
  struct action *a,
  struct action *b) {
  /* Merge sort implementation */
}

static struct action *msort__action(struct action *a) {
  /* Recursive merge sort */
}
```

**Go Equivalent - Custom Merge Sort**:
```go
func msort__action(list *action) *action {
    if list == nil || list.next == nil {
        return list
    }
    /* Split list in half */
    var slow, fast *action = list, list
    var prev *action
    for fast != nil && fast.next != nil {
        prev = slow
        slow = slow.next
        fast = fast.next.next
    }
    if prev != nil {
        prev.next = nil
    }
    
    left := msort__action(list)
    right := msort__action(slow)
    return merge__action(left, right)
}

func merge__action(a *action, b *action) *action {
    if a == nil { return b }
    if b == nil { return a }
    
    if actioncmp(a, b) < 0 {
        a.next = merge__action(a.next, b)
        return a
    } else {
        b.next = merge__action(a, b.next)
        return b
    }
}
```

### 14.2 Comparison Functions

**Pattern**: Comparison functions return int (-1, 0, 1 pattern).

**C Code**:
```c
int Symbolcmpp(const void *a, const void *b) {
    struct symbol *sa = *(struct symbol**)a;
    struct symbol *sb = *(struct symbol**)b;
    return strcmp(sa->name, sb->name);
}
```

**Go Equivalent**:
```go
func Symbolcmpp(a, b *symbol) int {
    if a.name < b.name {
        return -1
    }
    if a.name > b.name {
        return 1
    }
    return 0
}
```

### 14.3 Sort Interface

**Pattern**: Implement `sort.Interface` for Go's sort package.

**C Code**:
```c
qsort(lemp->symbols, lemp->nsymbol, sizeof(lemp->symbols[0]), Symbolcmpp);
```

**Go Equivalent**:
```go
type symbolSorter []*symbol

func (s symbolSorter) Len() int {
    return len(s)
}

func (s symbolSorter) Swap(i, j int) {
    s[i], s[j] = s[j], s[i]
}

func (s symbolSorter) Less(i, j int) bool {
    return Symbolcmpp(s[i], s[j]) < 0
}

/* Usage */
sort.Sort(symbolSorter(lemp.symbols))
```

---

## 15. HASH TABLES AND SYMBOL TABLES

### 15.1 Hash Table Structure

**Pattern**: Use Go maps instead of custom hash tables.

**C Code**:
```c
struct s_x4 {
    struct x4node **tbl;
    int size;
    struct x4node *first;
};

struct x4node {
    char *data;
    char key;
    struct x4node *next;
    struct x4node *plink;
};

static struct s_x4 *x4a_init() {
    struct s_x4 *new = malloc(sizeof(*new));
    new->size = 128;
    new->tbl = calloc(new->size, sizeof(new->tbl[0]));
    new->first = 0;
    return new;
}
```

**Go Equivalent**:
```go
type x4node struct {
    data string
    key  rune
    next *x4node
    plink *x4node
}

type s_x4 struct {
    tbl   map[rune]*x4node
    size  int
    first *x4node
}

func x4a_init() *s_x4 {
    return &s_x4{
        tbl: make(map[rune]*x4node),
        size: 128,
    }
}
```

### 15.2 Symbol Lookup

**Pattern**: Direct symbol table lookup via map or linear search.

**C Code**:
```c
struct symbol *Symbol_find(const char *name) {
    /* Linear search or hash table lookup */
}

struct symbol *Symbol_new(const char *name) {
    struct symbol *sp = malloc(sizeof(*sp));
    memset(sp, 0, sizeof(*sp));
    sp->name = malloc(strlen(name) + 1);
    strcpy(sp->name, name);
    return sp;
}
```

**Go Equivalent**:
```go
var symbolTable = make(map[string]*symbol)

func Symbol_find(name string) *symbol {
    return symbolTable[name]
}

func Symbol_new(name string) *symbol {
    sp := &symbol{
        name: name,
        /* Other fields zero-initialized */
    }
    symbolTable[name] = sp
    return sp
}
```

---

## 16. EXAMPLE: COMPLETE CONVERSION

Let's trace a complete function conversion from C to Go:

**C Source**:
```c
static struct config *newconfig(void)
{
  struct config *new;
  if( cfree ){
    new = cfree;
    cfree = cfree->next;
  }else{
    new = (struct config *)malloc(sizeof(struct config));
  }
  return new;
}

static void deleteconfig(struct config *old)
{
  old->next = cfree;
  cfree = old;
}

void Configlist_eat(struct config *cfp)
{
  struct config *nextcfp;
  for(; cfp; cfp=nextcfp){
    nextcfp = cfp->next;
    assert(cfp->fplp==NULL);
    assert(cfp->bplp==NULL);
    cfp->fws = NULL;
    deleteconfig(cfp);
  }
}
```

**Go Equivalent**:
```go
var cfree *config

func newconfig() *config {
    var new *config
    if cfree != nil {
        new = cfree
        cfree = cfree.next
    } else {
        new = &config{}
    }
    return new
}

func deleteconfig(old *config) {
    old.next = cfree
    cfree = old
}

func Configlist_eat(cfp *config) {
    var nextcfp *config
    for ; cfp != nil; cfp = nextcfp {
        nextcfp = cfp.next
        assert(cfp.fplp == nil, "cfp.fplp==nil")
        assert(cfp.bplp == nil, "cfp.bplp==nil")
        cfp.fws = nil
        deleteconfig(cfp)
    }
}
```

---

## 17. TESTING AND VALIDATION PATTERNS

### 17.1 Debug Output

**C Code**:
```c
#if 0
fprintf(stdout, "Debug output\n");
#endif
```

**Go Equivalent**:
```go
if false {  // #if 0
    fmt.Printf("Debug output\n")
}  // #endif
```

### 17.2 Assertions

**C Code**:
```c
#define MemoryCheck(X) if((X)==0){ \
  fprintf(stderr,"Out of memory\n"); exit(1); }
```

**Go Equivalent**:
```go
func assert(condition bool, message string) {
    if !condition {
        panic(message)
    }
}

// Usage:
assert(cfp != nil, "cfp!=nil")
```

---

## SUMMARY OF KEY PATTERNS

1. **Types**: C types map to Go in predictable ways
2. **Arrays**: Fixed arrays → fixed size; dynamic arrays → slices
3. **Strings**: Always use Go `string`, convert to `[]rune` for character ops
4. **Memory**: Use Go's automatic allocation; no malloc/free
5. **Macros**: Constants stay const; function-like macros become functions
6. **Pointers**: Direct mapping; `->` becomes `.`
7. **Functions**: Names stay the same; signatures update for Go types
8. **Globals**: Stay at package level; wrap in structs when possible
9. **Control Flow**: Direct 1:1 mapping for loops, switches, if/else
10. **CLI**: Use Go's `flag` package for command-line args
11. **I/O**: Use `os` and `fmt` packages
12. **Sorting**: Use `sort` package with custom comparators
13. **Templates**: Generate Go code instead of C code

