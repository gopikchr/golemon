# CLAUDE.md - Automated Upstream Porting Guide

This document provides complete instructions for porting upstream changes from the SQLite lemon parser to this Go port. It's designed to enable future Claude Code sessions to work autonomously on converting upstream commits.

---

## Table of Contents

1. [Overview & Porting Philosophy](#overview--porting-philosophy)
2. [Prerequisites & Setup](#prerequisites--setup)
3. [Identifying Work to Do](#identifying-work-to-do)
4. [Step-by-Step Porting Process](#step-by-step-porting-process)
5. [Testing Changes](#testing-changes)
6. [Committing Changes](#committing-changes)
7. [Troubleshooting & When to Collaborate](#troubleshooting--when-to-collaborate)
8. [For Future Claude Sessions](#for-future-claude-sessions)

---

## Overview & Porting Philosophy

### Project Purpose

This repository (`golemon`) is a Go port of the LEMON parser generator from SQLite. It tracks the upstream C implementation and ports changes as they occur.

### Porting Philosophy

**Principle: Similarity to C Code > Idiomatic Go**

This port prioritizes staying close to the original C code over making it fully idiomatic Go. The code is intentionally C-like Go code. This approach:

- Makes tracking upstream changes easier
- Allows for more mechanical conversions
- Reduces the risk of introducing bugs through over-abstraction
- Maintains structural similarity for future ports

### Key Philosophical Decisions

1. **Memory Management**: No `malloc()` equivalent - use Go's native allocation (`&struct{}`, `make()`, etc.)
2. **Arrays vs Slices**: Strike a balance - linked lists stay as pointers, dynamic arrays become slices
3. **String Handling**: All `char*` → `string`, character operations use `rune`
4. **CLI Functionality**: Adapt flags to Go's `flag` package, omit C-specific options when they don't make sense
5. **Macros**: Convert based on purpose:
   - Value macros → `const`
   - Function macros → functions
   - Conditional code → `if false { }` blocks or complete omission
6. **Naming**: Preserve C naming conventions (snake_case, etc.) for structural similarity

### Detailed Porting Reference

For detailed conversion patterns, see these comprehensive guides:

- **[C_TO_GO_PORTING_GUIDE.md](C_TO_GO_PORTING_GUIDE.md)** - Comprehensive 15-section guide with examples for every conversion pattern
- **[PORTING_SUMMARY.md](PORTING_SUMMARY.md)** - Executive summary organized by topic
- **[QUICK_REFERENCE.md](QUICK_REFERENCE.md)** - Cheat sheet for common conversions

These guides document specific patterns for:
- Type conversions (primitives, structs, enums, unions)
- Arrays, slices, and collections
- String and character handling
- Memory management
- Macros and preprocessor directives
- CLI flags and I/O
- Control flow and functions
- Templates and parsing
- Sorting and hash tables

---

## Prerequisites & Setup

### Required Environment

1. **This Repository** (`golemon`):
   ```bash
   /Users/zellyn/gh/p_gopikchr/golemon
   ```

2. **Upstream SQLite Repository** (sibling directory):
   ```bash
   /Users/zellyn/gh/p_gopikchr/sqlite
   ```

   The SQLite repository must be cloned as a sibling directory to `golemon`:
   ```bash
   cd /Users/zellyn/gh/p_gopikchr/
   git clone https://github.com/sqlite/sqlite.git
   ```

3. **Go Environment**:
   - Go 1.x or later installed
   - Ability to run `go build` and `go test`

### Directory Structure

```
/Users/zellyn/gh/p_gopikchr/
├── golemon/                    # This repository
│   ├── golemon.go             # Main Go implementation
│   ├── lempar.go.tpl          # Parser template (from lempar.c)
│   ├── intermediate/          # Intermediate conversion files
│   │   ├── lemon.c            # Copy of upstream lemon.c
│   │   ├── lemonc.go          # Mechanical C-to-Go conversion
│   │   └── lempar.c           # Copy of upstream lempar.c
│   └── CLAUDE.md              # This file
└── sqlite/                     # Upstream SQLite repository
    └── tool/
        ├── lemon.c            # Upstream lemon.c
        └── lempar.c           # Upstream lempar.c
```

### File Flow

The conversion process uses three stages:

1. **C Source** (`../sqlite/tool/lemon.c` and `lempar.c`)
2. **Intermediate** (`intermediate/lemon.c`, `intermediate/lemonc.go`, `intermediate/lempar.c`)
3. **Final Go** (`golemon.go`, `lempar.go.tpl`)

Changes flow: `upstream C` → `intermediate/` → `final Go files`

---

## Identifying Work to Do

### How the Upstream Tracking Works

A GitHub Action (`.github/workflows/check-upstream.yml`) runs daily to check for new commits in the upstream SQLite repository that affect `tool/lemon.c` or `tool/lempar.c`.

When new commits are found, the Action:
1. Creates one issue per commit
2. Labels each issue with `upstream-changes`
3. Sets the issue author to `github-actions`

### Finding Issues to Work On

To find issues that need porting:

```bash
gh issue list --label upstream-changes --state open
```

**Example output:**
```
18  OPEN  Port changes from upstream: Avoid calls to sprintf() in Lemon...  upstream-changes
19  OPEN  Port changes from upstream: Fix a goofy hash function in Lemon... upstream-changes
20  OPEN  Port changes from upstream: Update the "msort" function...       upstream-changes
```

### Issue Format

Each issue contains:

```
Title: Port changes from upstream: <first line of commit message>

Body:
New commit found in upstream SQLite repository that needs to be ported.

Commit: <full-commit-sha>
Author: <author-name>
Date: <commit-date>

Message:
<full-commit-message>

Original commit: https://github.com/sqlite/sqlite/commit/<full-commit-sha>
```

### Which Issue to Work On

**Rule: Process issues in order by issue number (oldest first)**

This ensures that changes are applied in chronological order, which is important for maintaining consistency with upstream.

```bash
# Get the oldest open issue
gh issue list --label upstream-changes --state open | tail -1
```

### Extracting Information from an Issue

From the issue body, extract:
- **Full commit SHA**: The 40-character hash
- **Short commit SHA**: First 10 characters (for commit messages)
- **Issue number**: For the "Closes #N" message

**Example:**
```bash
# View issue details
gh issue view 18

# Extract commit SHA (look for "Commit: <sha>" line in the output)
```

---

## Step-by-Step Porting Process

### Process Overview

For each upstream commit, you will:
1. Check out the upstream commit in the SQLite repo
2. Copy the C files to the intermediate directory
3. Review the changes
4. Analyze the intent and determine what action is needed
5. Port changes to the Go files (if appropriate)
6. Test the changes
7. Commit and close the issue

### Detailed Steps

#### Step 1: Check out the upstream commit

```bash
cd ../sqlite
git checkout <commit-sha>
```

**Purpose**: This ensures you're looking at the exact version of the files from the upstream commit.

**Example:**
```bash
cd ../sqlite
git checkout 8f06aed1dfce1801a5380f249e24be7f55767405
```

#### Step 2: Return to golemon directory

```bash
cd ../golemon
```

#### Step 3: Copy lemon.c to intermediate

```bash
cp ../sqlite/tool/lemon.c intermediate/
```

**Purpose**: This creates a snapshot of the upstream file that you'll use for comparison.

**Note**: The path is `tool/lemon.c` not `tools/lemon.c` (no 's' in tool).

#### Step 4: Copy lempar.c to intermediate

```bash
cp ../sqlite/tool/lempar.c intermediate/
```

**Purpose**: Same as step 3, for the parser template file.

#### Step 5: Review the changes

```bash
git diff
```

**What you're looking for:**
- Changes in `intermediate/lemon.c` - these need to be ported to `intermediate/lemonc.go` and `golemon.go`
- Changes in `intermediate/lempar.c` - these need to be ported to `lempar.go.tpl`

**Understanding the diff:**
- Red lines (deletions) = old version
- Green lines (additions) = new version from upstream
- The diff shows what changed in the upstream commit

**Example diff:**
```diff
diff --git a/intermediate/lemon.c b/intermediate/lemon.c
-  sprintf(buf, "/* %s:%d */", x->filename, x->lineno);
+  snprintf(buf, sizeof(buf), "/* %s:%d */", x->filename, x->lineno);
```
This shows a change from `sprintf` to `snprintf` for safety.

#### Step 6: Analyze the Intent and Determine Action

**CRITICAL**: Before making any changes to the Go code, stop and think about what the C change actually means and whether it applies to Go.

**Questions to ask yourself:**

1. **What problem does this C change solve?**
   - Security fix (buffer overflow, format string, etc.)?
   - Bug fix (logic error, edge case, etc.)?
   - Performance optimization?
   - Code refactoring or cleanup?
   - New feature or functionality?

2. **Does this problem exist in Go?**
   - **Memory safety**: Go has bounds checking, garbage collection
   - **Buffer overflows**: Go slices grow automatically
   - **NULL pointers**: Go has nil but with different semantics
   - **Format string attacks**: Go's fmt package is safe
   - **Algorithm/logic issues**: Usually still apply
   - **New features**: Usually still apply

3. **What action should I take?**

**Decision Matrix:**

| C Change Type | Does it Apply to Go? | Action |
|--------------|---------------------|---------|
| Security fix for C-specific issue | No | Update intermediate files only, add comment explaining why |
| Algorithm or logic change | Yes | Port the change to Go files |
| New feature/function | Yes | Port the change to Go files |
| Bug fix in shared logic | Yes | Port the change to Go files |
| Refactoring that improves clarity | Maybe | Port if it improves Go code too, otherwise document |
| Performance optimization | Maybe | Analyze if Go needs same optimization |
| **Unclear or complex change** | **Unknown** | **STOP and ask the user - explain what you found and request guidance** |

**Examples:**

**Example 1: sprintf → snprintf**
```diff
-  sprintf(buf, "/* %s:%d */", x->filename, x->lineno);
+  snprintf(buf, sizeof(buf), "/* %s:%d */", x->filename, x->lineno);
```

**Analysis:**
- **Problem**: Buffer overflow vulnerability in C
- **Applies to Go?**: No - Go's `fmt.Sprintf` returns a string, can't overflow
- **Action**:
  - Update `intermediate/lemon.c` and `intermediate/lemonc.go` (to track upstream)
  - **Do not change** `golemon.go` - it already uses `fmt.Sprintf`
  - Optionally add a comment:
    ```go
    // Note: Upstream uses snprintf for safety; Go's fmt.Sprintf is inherently safe
    ```

**Example 2: Fix off-by-one error**
```diff
-  for(i=0; i<n; i++){
+  for(i=0; i<=n; i++){
```

**Analysis:**
- **Problem**: Logic bug in loop bounds
- **Applies to Go?**: Yes - same logic error would exist
- **Action**: Port the change to all Go files

**Example 3: Make sort stable**
```diff
Commit message: "Update the msort function to make it stable"
```

**Analysis:**
- **Problem**: Sort wasn't preserving order of equal elements
- **Applies to Go?**: Unclear - need to check our implementation
- **Action**:
  - Check if we use custom sort or Go's `sort` package
  - Check if Go's sort is already stable (it's not by default, but `sort.Stable` exists)
  - **Stop and ask user**: "Need to verify our sort implementation and decide approach"

**Example 4: Add new validation function**
```diff
+ void validateState(struct state *s) {
+   if (s->nrule == 0) error("empty state");
+ }
```

**Analysis:**
- **Problem**: Adding validation that was missing
- **Applies to Go?**: Yes - same validation makes sense
- **Action**: Port the new function to Go

---

**⚠️ IMPORTANT: When in Doubt, Discuss with the User**

If you're uncertain about how to handle a change, **STOP and collaborate with the user**. It's better to ask than to make incorrect assumptions. Explain:
- What the upstream change does
- Why you're uncertain about how to handle it
- What options you see for how to proceed

The user can provide context, help analyze whether the change applies to Go, and guide the approach. See the "Troubleshooting & When to Collaborate" section for detailed guidance on when and how to ask for help.

#### Step 7: Port changes from lempar.c to lempar.go.tpl

**Process:**
1. Look at the diff in `intermediate/lempar.c`
2. Find the equivalent code in `lempar.go.tpl`
3. Apply the changes idiomatically for this conversion (not idiomatically for Go, but idiomatically for this C-to-Go conversion)

**Key considerations:**
- Use the porting guides to understand how to convert C constructs
- Maintain the structural similarity to the C code
- The template uses Go syntax but follows C patterns
- Look for Go equivalents of C library functions (see QUICK_REFERENCE.md)

**Example:**
```c
// C code in lempar.c
for(i=0; i<n; i++){
  printf("%s", names[i]);
}
```

```go
// Go code in lempar.go.tpl (note: preserving C-like style)
for i := 0; i < n; i++ {
  fmt.Print(names[i])
}
```

**What if there are no changes?**
If `git diff` shows no changes to `intermediate/lempar.c`, skip this step.

#### Step 8: Port changes from lemon.c to lemonc.go to golemon.go

**This is a two-stage process:**

**Stage 1: Update intermediate/lemonc.go**
1. Look at the diff in `intermediate/lemon.c`
2. Find the equivalent code in `intermediate/lemonc.go`
3. Apply a mechanical C-to-Go conversion
4. This file is a more literal translation of the C code

**Stage 2: Update golemon.go**
1. Look at the changes you made to `intermediate/lemonc.go`
2. Find the equivalent code in `golemon.go`
3. Apply the changes, adapting for any structural differences
4. This file is the final, working Go implementation

**Why two stages?**
- `lemonc.go` serves as an intermediate step that's closer to C
- `golemon.go` is the actual implementation used by the program
- This two-stage process makes it easier to track complex changes

**Key considerations:**
- Follow the porting patterns in C_TO_GO_PORTING_GUIDE.md
- Preserve function names and structure where possible
- Use Go's standard library equivalents for C library functions
- Maintain the same logic flow as the C code

**Example workflow:**
```c
// Change in lemon.c
-  sprintf(buf, "/* %s:%d */", x->filename, x->lineno);
+  snprintf(buf, sizeof(buf), "/* %s:%d */", x->filename, x->lineno);
```

```go
// Update in lemonc.go (mechanical conversion)
-  buf = fmt.Sprintf("/* %s:%d */", x.filename, x.lineno)
+  buf = fmt.Sprintf("/* %s:%d */", x.filename, x.lineno)
// Note: In Go, Sprintf is already safe, so the change is a no-op
```

```go
// Update in golemon.go (same logic)
-  buf = fmt.Sprintf("/* %s:%d */", x.filename, x.lineno)
+  buf = fmt.Sprintf("/* %s:%d */", x.filename, x.lineno)
// Note: Same as lemonc.go in this case
```

**What if there are no changes?**
If `git diff` shows no changes to `intermediate/lemon.c`, skip this step.

#### Step 9: Verify and stage all changes

Before testing, review what you've changed:

```bash
git diff
```

**Expected changes:**
- `intermediate/lemon.c` (copied from upstream)
- `intermediate/lempar.c` (copied from upstream)
- `intermediate/lemonc.go` (ported from lemon.c)
- `golemon.go` (ported from lemonc.go)
- `lempar.go.tpl` (ported from lempar.c)

All five files should typically be modified (unless the upstream commit only touched one of the C files).

---

## Testing Changes

Before committing, verify that the changes work correctly.

### Build Test

```bash
go build
```

**Expected result:** Clean build with no errors.

**If the build fails:**
- Review the error messages
- Check your porting for syntax errors
- Compare against the porting guides
- See "Troubleshooting" section below

### Run Tests (if they exist)

```bash
go test
```

**Expected result:** All tests pass.

**If tests fail:**
- Review the test output
- Check if the logic was ported correctly
- Compare the C and Go implementations side-by-side
- See "Troubleshooting" section below

### Smoke Test (optional but recommended)

If time permits, run the program manually to ensure basic functionality:

```bash
./golemon --help
```

or test with a simple grammar file if available.

### What if there are no tests?

Many upstream changes are small (comments, formatting, minor bug fixes). If:
- The build succeeds
- The changes are minimal
- The porting is straightforward

You can proceed to commit.

---

## Committing Changes

### Commit Message Format

**CRITICAL**: Use this exact format to auto-close the issue:

```
track upstream commit <short-sha>

https://github.com/sqlite/sqlite/commit/<full-sha>

Closes #<issue-number>
```

**Parameters:**
- `<short-sha>`: First 10 characters of the commit SHA
- `<full-sha>`: Full 40-character commit SHA
- `<issue-number>`: The GitHub issue number

**Example:**
```
track upstream commit 8f06aed1df

https://github.com/sqlite/sqlite/commit/8f06aed1dfce1801a5380f249e24be7f55767405

Closes #18
```

### Files to Include

Typically, include all five files:
```bash
git add intermediate/lemon.c
git add intermediate/lempar.c
git add intermediate/lemonc.go
git add golemon.go
git add lempar.go.tpl
```

**Exception**: If the upstream commit only modified one C file, you may only have changes in 2-3 files. Include whatever actually changed.

### Creating the Commit

**Using a heredoc for proper formatting:**

```bash
git commit -m "$(cat <<'EOF'
track upstream commit 8f06aed1df

https://github.com/sqlite/sqlite/commit/8f06aed1dfce1801a5380f249e24be7f55767405

Closes #18
EOF
)"
```

**Why use a heredoc?**
- Ensures proper multi-line formatting
- Preserves the blank line between the title and body
- The blank line is required for GitHub to parse "Closes #N"

### Pushing the Commit

```bash
git push
```

**What happens next:**
- GitHub automatically closes the issue (because of "Closes #18")
- The commit appears in the repository history
- The next Claude session can pick up the next issue

### Verification

After pushing, verify:
```bash
gh issue view <issue-number>
```

The issue should show as "CLOSED" with the commit linked.

---

## Troubleshooting & When to Collaborate

### When Automatic Conversion is Unclear

Not all C-to-Go conversions are mechanical. When you encounter situations that require judgment or deeper understanding, **collaborate with the user**.

### Red Flags - Stop and Ask

**Scenario 1: Semantic Changes**

If the upstream commit changes the behavior or algorithm (not just syntax), pause and discuss.

**Example:**
```
Issue: "Update the msort function in Lemon so that it works with lists of any..."
```

This suggests a change to the sorting algorithm. Questions to ask:
- Did we port the sort using Go's built-in sort?
- Is Go's sort already stable (making this change unnecessary)?
- Do we need to modify our implementation?

**Action:** Ask the user:
```
"This upstream change modifies the sorting algorithm to make it stable.
I see we have a custom merge sort implementation in golemon.go.

Questions:
1. Should I check if our implementation is already stable?
2. Do you want me to adopt the upstream changes directly?
3. Should we switch to Go's sort.Stable if appropriate?"
```

**Scenario 2: New Functions or Major Refactoring**

If the upstream commit adds entirely new functions or significantly refactors existing code, discuss the approach.

**Action:** Describe what changed and ask how to proceed:
```
"The upstream commit adds a new function 'xyz()' that does ABC.
This requires adding ~50 lines of new code.

Should I:
1. Port it directly following our C-to-Go patterns?
2. Use a Go standard library equivalent if one exists?
3. Review the need for this function in the Go context?"
```

**Scenario 3: Macro or Preprocessor Changes**

If the upstream commit modifies complex macros or conditional compilation, discuss the intent.

**Action:** Explain the C macro and ask about the Go equivalent:
```
"The upstream commit changes the macro XYZ from:
  #define XYZ(a,b) (a > b ? a : b)
to:
  #define XYZ(a,b) ((a) > (b) ? (a) : (b))

This adds parentheses for safety. Our Go version is:
  func XYZ(a, b int) int { if a > b { return a }; return b }

Do we need to change anything, or is this purely a C safety issue?"
```

**Scenario 4: Build or Test Failures**

If your port causes build errors or test failures that aren't obvious to fix:

**Action:** Share the error and your analysis:
```
"After porting the changes, I get this build error:
  [error message]

I ported the C code:
  [show C code]
as:
  [show Go code]

This follows pattern X from the porting guide, but I'm getting an error.
Possible issues:
1. [hypothesis 1]
2. [hypothesis 2]

Should I try approach [A] or [B]?"
```

### Green Flags - Proceed Autonomously

You can proceed without asking when:

1. **Simple syntax changes**: `sprintf` → `snprintf`, comment changes, formatting
2. **Mechanical conversions**: Following clear patterns from the porting guides
3. **Type-safe equivalents**: C safety fixes that Go already handles
4. **Build succeeds and tests pass**: The porting worked correctly

### Collaboration Guidelines

**Be proactive but not presumptuous:**
- ✅ Do: Explain what you found and propose options
- ✅ Do: Show your reasoning and analysis
- ✅ Do: Offer 2-3 concrete approaches
- ❌ Don't: Make major architectural decisions without discussion
- ❌ Don't: Skip difficult conversions hoping they'll work
- ❌ Don't: Commit broken code to "fix it later"

**Document your decisions:**

When you make a judgment call (after user confirmation), add a comment:

```go
// Port note: Upstream added stable sorting in commit 8f06aed1df.
// Go's sort.Stable provides this natively, so no changes needed.
```

This helps future maintainers understand why the Go code diverges from C.

### Common Issues and Solutions

| Issue | Likely Cause | Solution |
|-------|-------------|----------|
| Build error: undefined function | Forgot to port a new function | Check if upstream added a function; port it |
| Build error: type mismatch | Incorrect type conversion | Review porting guides for correct type mapping |
| Test failure: wrong output | Logic error in porting | Compare C and Go line-by-line for the affected function |
| Diff shows no changes | Upstream change doesn't apply to Go | Discuss with user; may be C-specific (e.g., memory safety) |
| Can't find equivalent code | Code was refactored in Go version | Search for similar logic; discuss with user if unclear |

---

## For Future Claude Sessions

### Automated Workflow

When asked to "port upstream changes" or "process pending issues", follow this workflow:

#### 1. **List open issues**
```bash
gh issue list --label upstream-changes --state open
```

#### 2. **Select the oldest issue**
```bash
# Get the issue with the smallest number
gh issue list --label upstream-changes --state open | tail -1
```

#### 3. **Extract information**
```bash
gh issue view <issue-number>
```
Extract:
- Commit SHA (full)
- Short SHA (first 10 chars)
- Issue number

#### 4. **Follow the porting process**
- Steps 1-9 from "Step-by-Step Porting Process" section
  - Pay special attention to Step 6 (Analyze Intent) - determine if the change applies to Go
- Test the changes
- Commit with proper format

#### 5. **Verify issue is closed**
```bash
gh issue view <issue-number>
```

#### 6. **Repeat for next issue**
Continue with the next oldest issue until:
- All issues are processed, or
- You encounter an issue that needs user collaboration

### Working on Multiple Issues

**Approach: One at a time, in order**

Process issues sequentially:
1. Port issue #18
2. Commit and verify
3. Port issue #19
4. Commit and verify
5. Continue...

**Do not batch commits**: Each upstream commit should result in one commit in this repository. This maintains the 1:1 correspondence with upstream.

### When to Stop and Ask

Stop automatic processing and ask the user when you encounter:
- An issue that requires semantic judgment (see "Troubleshooting" section)
- Build or test failures you can't resolve
- An upstream change that fundamentally conflicts with the Go implementation
- More than 3 consecutive issues (check in with user on progress)

### Communication with User

After processing each issue, provide a brief update:

**Example:**
```
✓ Issue #18 completed: Ported sprintf → snprintf changes
  - Changes were mechanical (Go's fmt.Sprintf already safe)
  - Build successful, no tests affected
  - Committed: track upstream commit 8f06aed1df

✓ Issue #19 completed: Ported hash function fix
  - Updated hash calculation in findState()
  - Build successful, tests pass
  - Committed: track upstream commit a1b2c3d4ef

⚠ Issue #20 requires discussion: Sorting algorithm change
  - Upstream made sort stable
  - Need to verify if our implementation needs updates
  - Pausing for guidance
```

This keeps the user informed and shows progress.

### Success Criteria

You've successfully completed the porting workflow when:
1. ✅ All `upstream-changes` issues are closed
2. ✅ All commits follow the correct format
3. ✅ Build succeeds (`go build` works)
4. ✅ Tests pass (`go test` works, if tests exist)
5. ✅ Each upstream commit has a corresponding commit in this repo

### Long-term Maintenance

**This process repeats indefinitely:**
- GitHub Action creates new issues as upstream changes occur
- Future Claude sessions (or maintainers) process issues using this guide
- The Go port stays in sync with upstream SQLite lemon parser

**Keeping this guide updated:**
If you discover new porting patterns or edge cases, consider updating:
- This CLAUDE.md file (for process improvements)
- The detailed porting guides (for new conversion patterns)
- The troubleshooting section (for new common issues)

---

## Quick Command Reference

```bash
# Find issues to work on
gh issue list --label upstream-changes --state open

# View issue details
gh issue view <number>

# Port workflow
cd ../sqlite && git checkout <commit-sha>
cd ../golemon
cp ../sqlite/tool/lemon.c intermediate/
cp ../sqlite/tool/lempar.c intermediate/
git diff

# After porting...
go build
go test

# Commit
git add intermediate/lemon.c intermediate/lempar.c intermediate/lemonc.go golemon.go lempar.go.tpl
git commit -m "$(cat <<'EOF'
track upstream commit <short-sha>

https://github.com/sqlite/sqlite/commit/<full-sha>

Closes #<issue-number>
EOF
)"
git push

# Verify
gh issue view <number>  # Should show CLOSED
```

---

## Additional Resources

- **[C_TO_GO_PORTING_GUIDE.md](C_TO_GO_PORTING_GUIDE.md)** - Comprehensive conversion patterns with examples
- **[PORTING_SUMMARY.md](PORTING_SUMMARY.md)** - Executive summary of porting philosophy
- **[QUICK_REFERENCE.md](QUICK_REFERENCE.md)** - Quick lookup for common conversions
- **[README.md](README.md)** - Project overview and usage
- **Upstream SQLite Lemon**: https://github.com/sqlite/sqlite/tree/master/tool

---

**Last Updated**: 2025-10-25
**Version**: 1.0
