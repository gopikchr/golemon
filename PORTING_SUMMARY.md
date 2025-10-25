# Lemon Parser C-to-Go Port: Comprehensive Analysis

## Executive Summary

This document provides a complete analysis of how the LEMON parser generator (from SQLite) has been ported from C to Go, including detailed conversion patterns for future changes.

Generated: October 25, 2025
Source: Analysis of golemon.go (5718 lines), lemonc.go, lempar.go.tpl

## Key Files Analyzed

1. **golemon.go** - Main Go implementation (5,718 lines)
2. **intermediate/lemonc.go** - Intermediate version showing conversion patterns
3. **intermediate/lemon.c** - Original C source for reference
4. **lempar.go.tpl** - Go parser template (ported from lempar.c)
5. **notes.md** - Documentation of changes from upstream commits

## Detailed Porting Patterns

A comprehensive 1,700+ line guide (`C_TO_GO_PORTING_GUIDE.md`) has been created documenting:

### 1. Type Conversions (Section 1)
- Primitive type mappings: `int`, `char*`, `void*`, etc.
- Struct conversions with field name preservation
- Union type handling via discriminated structs
- Type aliases using `type X = Y` pattern

### 2. Arrays vs Slices (Section 2)
- Fixed arrays vs dynamic slices distinction
- Linked list pointer patterns (unchanged)
- Array growth via `append()` instead of `realloc()`
- Set data structures using `map[int]bool`

### 3. String and Character Handling (Section 3)
- All `char*` becomes Go `string`
- Character loops use `[]rune` conversion
- String functions: `strlen()` → `len()`, `strcpy()` → `=`, etc.
- Character classification: C macros → `unicode` package functions
- String operations: `strings` package used throughout

### 4. Memory Management (Section 4)
- Dynamic allocation: `malloc()` → `&struct{}` or `make()`
- Pointer handling: Direct mapping with implicit dereferencing
- NULL becomes `nil`
- Object pools preserved using linked lists of pre-allocated structures
- No explicit `free()` calls needed (Go's GC)

### 5. Macros/Defines (Section 5)
- Value macros: `#define NAME value` → `const NAME = value`
- Function macros: `#define MACRO(x)` → `func MACRO(x)`
- Conditional compilation: `#ifdef` → Go conditional logic or omitted
- Preprocessor text replacement: Custom `defines` struct with regex matching

### 6. CLI Flags (Section 6)
- C `argc`/`argv` → Go `flag` package
- Custom flag types via `flag.Value` interface
- Global state wrapped in `lemon` struct
- Support for `-D` and `-U` options via custom flag type

### 7. Naming Conventions (Section 7)
- Function names preserved exactly from C
- Type names: lowercase internal, mixed external
- Field names keep C format (snake_case preserved)
- Constants: Exact C naming

### 8. Control Flow (Section 8)
- If/else: Direct 1:1 mapping
- For loops: Identical structure
- While loops: `while(x)` → `for x { }`
- Linked list iteration: Unchanged pointer traversal
- Switch statements: Go's implicit break instead of explicit C break
- Fallthrough uses explicit `fallthrough` keyword

### 9. Functions (Section 9)
- Direct function mappings
- Go supports multiple return values directly
- Function pointers: Explicit type definitions
- Variadic arguments: `...interface{}` for flexible arg passing
- No forward declarations needed

### 10. Global Variables (Section 10)
- Package-level `var` declarations
- Pointer-to-pointer state preserved
- Counters and freelists use same patterns
- Large state wrapped in main `lemon` struct

### 11. File I/O (Section 11)
- `fopen()` → `os.OpenFile()`
- `fread()` → `os.ReadFile()` or `bufio.Reader`
- `fprintf()` → `fmt.Fprintf()`
- Automatic cleanup via `defer fp.Close()`

### 12. Parser Template (Section 12)
- Generates Go code instead of C code
- Type definitions instead of macros
- Go struct instead of C union for stack
- Methods on `yyParser` receiver

### 13. Recursive Descent Parsing (Section 13)
- State machine with enum converted to `const iota`
- Token loop with character-by-character processing
- Rune slicing for token extraction

### 14. Sorting and Comparisons (Section 14)
- Custom merge sort preserved (same algorithm)
- Comparison functions return int (-1/0/1)
- `sort.Interface` implementation for Go's `sort` package

### 15. Hash Tables and Symbol Tables (Section 15)
- C hash tables → Go `map[string]*symbol`
- Symbol lookup via map instead of custom hash table
- Symbol creation and registration in maps

## Mechanical Conversion Checklist

When converting C code changes to Go:

### Type System
- [ ] Map C types to Go equivalents
- [ ] Convert `#define` constants to `const`
- [ ] Convert function-like macros to functions
- [ ] Convert struct pointers to Go struct literals

### Memory
- [ ] Remove all `malloc()` → use direct allocation
- [ ] Remove all `free()` calls (unused in Go)
- [ ] Convert `memcpy()` → `copy()` for slices
- [ ] Replace `NULL` with `nil`

### Arrays/Collections
- [ ] C arrays with size counters → Go slices (drop size counters)
- [ ] Linked lists → keep pointer pattern
- [ ] Array growth → use `append()`
- [ ] Sets → use `map[int]bool`

### Strings
- [ ] `char*` → `string`
- [ ] `strlen()` → `len()`
- [ ] String comparison → `==` or `strings` package
- [ ] Character loops → `[]rune()` conversion

### Functions
- [ ] Keep function names exactly
- [ ] Update parameter types
- [ ] Use multiple return values instead of out-parameters
- [ ] Keep logic identical

### File I/O
- [ ] `fopen()` → `os.OpenFile()`
- [ ] `fprintf()` → `fmt.Fprintf()`
- [ ] Add `defer Close()`
- [ ] Handle errors with `if err != nil`

### Control Flow
- [ ] For loops: identical C syntax works in Go
- [ ] While loops: use `for cond {}`
- [ ] Switch: remove `break` statements (Go has implicit break)
- [ ] Linked list iteration: keep pointer pattern

## Known Deviations from C

1. **Memory Management**: No explicit malloc/free (Go GC handles it)
2. **Platform-Specific Code**: Not needed (Go is cross-platform)
3. **Stack Overflow Protection**: Different mechanisms (Go vs C)
4. **Compiler Warnings**: Different warning systems (C vs Go)
5. **Performance Optimizations**: Some C-specific optimizations omitted

## Files Modified Relative to Upstream

- **lemon.c**: Small changes; mostly direct port
- **lempar.c**: Converted to Go template (lempar.go.tpl)
- **build.sh**: Updated to invoke Go compiler
- **notes.md**: Documents upstream commits and deviations

## Testing and Validation

The port maintains compatibility with the original LEMON grammar format. Key considerations:

1. **Grammar Files**: `.y` grammar files are parsed identically
2. **Output**: Generated parser code is now Go instead of C
3. **Functionality**: All parser generator features preserved
4. **Performance**: Go version should have comparable performance

## Future Maintenance

To maintain this port as LEMON evolves:

1. Check upstream commits for lemon.c changes
2. Use the conversion patterns in `C_TO_GO_PORTING_GUIDE.md`
3. Keep variable names and logic identical when possible
4. Test with existing grammar files to verify correctness
5. Update both golemon.go and intermediate/lemonc.go

## Quick Reference: Most Common Patterns

```c
// C Pattern
struct symbol *sp = malloc(sizeof(*sp));
sp->name = x;
for(rp = head; rp != NULL; rp = rp->next) { /* ... */ }

// Go Equivalent
sp := &symbol{}
sp.name = x
for rp := head; rp != nil; rp = rp.next { /* ... */ }
```

```c
// C Pattern
fprintf(fp, "const int x = %d;\n", val);
char *name = malloc(strlen(x) + 1);
strcpy(name, x);

// Go Equivalent
fmt.Fprintf(fp, "const int x = %d;\n", val)
name := x  // Direct assignment
```

## Document Organization

This summary document provides an overview. For detailed patterns with examples:
- See `C_TO_GO_PORTING_GUIDE.md` (1,700+ lines)
- Sections 1-15 cover all major conversion categories
- Each section has C code examples and Go equivalents
- Includes complete worked examples

## Additional Resources

- `/Users/zellyn/gh/p_gopikchr/golemon/README.md` - Basic overview
- `/Users/zellyn/gh/p_gopikchr/golemon/notes.md` - Upstream commit tracking
- `/Users/zellyn/gh/p_gopikchr/golemon/golemon.go` - Main implementation
- `/Users/zellyn/gh/p_gopikchr/golemon/lempar.go.tpl` - Parser template

---

**This guide enables future porters to mechanically convert C changes to Go while maintaining code correctness and consistency.**
