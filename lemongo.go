package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

/*
** This file contains all sources (including headers) to the LEMON
** LALR(1) parser generator.  The sources have been combined into a
** single file to make it easy to include LEMON in the source tree
** and Makefile of another program.
**
** The author of this program disclaims copyright.
 */

func isalnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func islower(r rune) bool {
	return unicode.IsLetter(r) && !unicode.IsUpper(r)
}

// var MAXRHS = 5 /* Set low to exercise exception code */
var MAXRHS = 1000

var showPrecedenceConflict bool

func SetFind(s map[int]bool, e int) bool {
	return s[e]
}

/********** From the file "struct.h" *************************************/
/*
** Principal data structures for the LEMON parser generator.
 */

/* Symbols (terminals and nonterminals) of the grammar are stored
** in the following: */
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

type symbol struct {
	name       string       /* Name of the symbol */
	index      int          /* Index number for this symbol */
	typ        symbol_type  /* Symbols are all either TERMINALS or NTs */
	rule       *rule        /* Linked list of rules of this (if an NT) */ //? slice?
	fallback   *symbol      /* fallback token in case this token doesn't parse */
	prec       int          /* Precedence if defined (-1 otherwise) */
	assoc      e_assoc      /* Associativity if precedence is defined */
	firstset   map[int]bool /* First-set for all rules of this symbol */
	lambda     bool         /* True if NT and can generate an empty string */
	useCnt     int          /* Number of times used */
	destructor string       /* Code which executes whenever this symbol is
	 ** popped from the stack during error processing */
	destLineno int /* Line number for start of destructor.  Set to
	 ** -1 for duplicate destructors. */
	datatype string /* The data type of information held by this
	 ** object. Only used if type==NONTERMINAL */
	dtnum int /* The data type number.  In the parser, the value
	 ** stack is a union.  The .yy%d element of this
	 ** union is the correct data type for this object */
	bContent bool /* True if this symbol ever carries content - if
	 ** it is ever more than just syntax */
	/* The following fields are used by MULTITERMINALs only */
	subsym []*symbol /* Array of constituent symbols */
}

/* Each production rule in the grammar is stored in the following
** structure.  */
type rule struct {
	lhs         *symbol   /* Left-hand side of the rule */
	lhsalias    string    /* Alias for the LHS ("" if none) */
	lhsStart    bool      /* True if left-hand side is the start symbol */
	ruleline    int       /* Line number for the rule */
	rhs         []*symbol /* The RHS symbols */
	rhsalias    []string  /* An alias for each RHS symbol (empty if none) */
	line        int       /* Line number at which code begins */
	code        string    /* The code executed when this rule is reduced */
	codePrefix  string    /* Setup code before code[] above */
	codeSuffix  string    /* Breakdown code after code[] above */
	precsym     *symbol   /* Precedence symbol for this rule */
	index       int       /* An index number for this rule */
	iRule       int       /* Rule number as used in the generated tables */
	noCode      bool      /* True if this rule has no associated C code */
	codeEmitted bool      /* True if the code has been emitted already */
	canReduce   bool      /* True if this rule is ever reduced */
	doesReduce  bool      /* Reduce actions occur after optimization */
	neverReduce bool      /* Reduce is theoretically possible, but prevented by actions or other outside implementation */
	nextlhs     *rule     /* Next rule with the same LHS */
	next        *rule     /* Next rule in the global list */
}

/* A configuration is a production rule of the grammar together with
** a mark (dot) showing how much of that rule has been processed so far.
** Configurations also contain a follow-set which is a list of terminal
** symbols which are allowed to immediately follow the end of the rule.
** Every configuration is recorded as an instance of the following: */
type cfgstatus int

const (
	COMPLETE cfgstatus = iota
	INCOMPLETE
)

type config struct {
	rp     *rule        /* The rule upon which the configuration is based */
	dot    int          /* The parse point */
	fws    map[int]bool /* Follow-set for this configuration only */
	fplp   *plink       /* Follow-set forward propagation links */
	bplp   *plink       /* Follow-set backwards propagation links */
	stp    *state       /* Pointer to state which contains this */
	status cfgstatus    /* used during followset and shift computations */
	next   *config      /* Next configuration in the state */
	bp     *config      /* The next basis configuration */
}

type e_action int

const (
	SHIFT e_action = iota
	ACCEPT
	REDUCE
	ERROR
	SSCONFLICT  /* A shift/shift conflict */
	SRCONFLICT  /* Was a reduce, but part of a conflict */
	RRCONFLICT  /* Was a reduce, but part of a conflict */
	SH_RESOLVED /* Was a shift.  Precedence resolved conflict */
	RD_RESOLVED /* Was reduce.  Precedence resolved conflict */
	NOT_USED    /* Deleted by compression */
	SHIFTREDUCE /* Shift first, then reduce */
)

type stateOrRuleUnion struct {
	stp *state /* The new state, if a shift */
	rp  *rule  /* The rule, if a reduce */
}

/* Every shift or reduce operation is stored as one of the following */
type action struct {
	sp      *symbol /* The look-ahead symbol */
	typ     e_action
	x       stateOrRuleUnion
	spOpt   *symbol /* SHIFTREDUCE optimization to this symbol */
	next    *action /* Next action for this state */
	collide *action /* Next action with the same hash */
	index   int     /// creation index, used for actioncmp
}

/* Each state of the generated parser's finite state machine
** is encoded as an instance of the following structure. */
type state struct {
	bp          *config /* The basis configurations for this state */
	cfp         *config /* All configurations in this set */
	statenum    int     /* Sequential number for this state */
	ap          *action /* List of actions for this state */
	nTknAct     int     /* Number of actions on terminals and nonterminals */
	nNtAct      int
	iTknOfst    int /* yyaction[] offset for terminals and nonterms */
	iNtOfst     int
	iDfltReduce int   /* Default action is to REDUCE by this rule */
	pDfltReduce *rule /* The default REDUCE rule. */
	autoReduce  bool  /* True if this is an auto-reduce state */
}

const NO_OFFSET = -2147483647

/* A followset propagation link indicates that the contents of one
** configuration followset should be propagated to another whenever
** the first changes. */
type plink struct {
	cfp  *config /* The configuration to which linked */
	next *plink  /* The next propagate link */
}

/* The state vector for the entire parser generator is recorded as
** follows.  (LEMON uses no global variables and makes little use of
** static variables.  Fields in the following structure can be thought
** of as begin global variables in the program.) */
type lemon struct {
	sorted            []*state  /* Table of states sorted by state number */
	rule              *rule     /* List of all rules */
	startRule         *rule     /* First rule */
	nstate            int       /* Number of states */
	nxstate           int       /* nstate with tail degenerate states removed */
	nrule             int       /* Number of rules */
	nruleWithAction   int       /* Number of rules with actions */
	nsymbol           int       /* Number of terminal and nonterminal symbols */
	nterminal         int       /* Number of terminal symbols */
	minShiftReduce    int       /* Minimum shift-reduce action value */
	errAction         int       /* Error action value */
	accAction         int       /* Accept action value */
	noAction          int       /* No-op action value */
	minReduce         int       /* Minimum reduce action */
	maxAction         int       /* Maximum action value of any kind */
	symbols           []*symbol /* Sorted array of pointers to symbols */
	errorcnt          int       /* Number of errors */
	errsym            *symbol   /* The error symbol */
	wildcard          *symbol   /* Token that matches anything */
	name              string    /* Name of the generated parser */
	arg               string    /* Declaration of the 3rd argument to parser */
	ctx               string    /* Declaration of 2nd argument to constructor */
	tokentype         string    /* Type of terminal symbols in the parser stack */
	vartype           string    /* The default type of non-terminal symbols */
	start             string    /* Name of the start symbol for the grammar */
	stacksize         string    /* Size of the parser stack */
	include           string    /* Code to put at the start of the C file */
	error             string    /* Code to execute when an error is seen */
	overflow          string    /* Code to execute on a stack overflow */
	failure           string    /* Code to execute on parser failure */
	accept            string    /* Code to execute when the parser excepts */
	extracode         string    /* Code appended to the generated file */
	tokendest         string    /* Code to execute to destroy token data */
	vardest           string    /* Code for the default non-terminal destructor */
	filename          string    /* Name of the input file */
	outname           string    /* Name of the current output file */
	tokenprefix       string    /* A prefix added to token names in the .h file */
	nconflict         int       /* Number of parsing conflicts */
	nactiontab        int       /* Number of entries in the yyaction[] table */
	nlookaheadtab     int       /* Number of entries in yylookahead[] */
	tablesize         int       /* Total table size of all tables in bytes */
	basisflag         bool      /* Print only basis configurations */
	printPreprocessed bool      /* Show preprocessor output on stdout */
	has_fallback      bool      /* True if any %fallback is seen in the grammar */
	nolinenosflag     bool      /* True if #line statements should not be printed */
	argv0             string    /* Name of the program */
}

/**************** From the file "table.h" *********************************/
/*
** All code in this file has been automatically generated
** from a specification in the file
**              "table.q"
** by the associative array code building program "aagen".
** Do not edit this file!  Instead, edit the specification
** file, then rerun aagen.
 */
/*
** Code for processing tables in the LEMON parser generator.
 */
/* Routines for handling a strings */

/****************** From the file "action.c" *******************************/
/*
** Routines processing parser actions in the LEMON parser generator.
 */

var actionIndex = 0

/* Allocate a new parser action */
func Action_new() *action {
	actionIndex++
	return &action{
		index: actionIndex,
	}
}

/* Compare two actions for sorting purposes.  Return negative, zero, or
** positive if the first action is less than, equal to, or greater than
** the first
 */
func actioncmp(ap1, ap2 *action) int {
	rc := ap1.sp.index - ap2.sp.index
	if rc == 0 {
		rc = int(ap1.typ) - int(ap2.typ)
	}
	if rc == 0 && (ap1.typ == REDUCE || ap1.typ == SHIFTREDUCE) {
		rc = ap1.x.rp.index - ap2.x.rp.index
	}
	if rc == 0 {
		rc = ap2.index - ap1.index
	}
	return rc
}

/* Sort parser actions */
func Action_sort(ap *action) *action {
	return msort__action(ap)
}

func Action_add(app **action, typ e_action, sp *symbol, stateOrRule stateOrRuleUnion) {
	newaction := Action_new()
	newaction.next = *app
	*app = newaction
	newaction.typ = typ
	newaction.sp = sp
	newaction.spOpt = nil
	newaction.x = stateOrRule
}

/********************** New code to implement the "acttab" module ***********/
/*
** This module implements routines use to construct the yy_action[] table.
 */

/*
** The state of the yy_action table under construction is an instance of
** the following structure.
**
** The yy_action table maps the pair (state_number, lookahead) into an
** action_number.  The table is an array of integers pairs.  The state_number
** determines an initial offset into the yy_action array.  The lookahead
** value is then added to this initial offset to get an index X into the
** yy_action array. If the aAction[X].lookahead equals the value of the
** of the lookahead input, then the value of the action_number output is
** aAction[X].action.  If the lookaheads do not match then the
** default action for the state_number is returned.
**
** All actions associated with a single state_number are first entered
** into aLookahead[] using multiple calls to acttab_action().  Then the
** actions for that single state_number are placed into the aAction[]
** array with a single call to acttab_insert().  The acttab_insert() call
** also resets the aLookahead[] array in preparation for the next
** state number.
 */
type lookahead_action struct {
	lookahead int /* Value of the lookahead token */
	action    int /* Action to take on the given lookahead */
}

type acttab struct {
	nAction         int                /* Number of used slots in aAction[] */
	nActionAlloc    int                /* Slots allocated for aAction[] */
	aAction         []lookahead_action /* The yyaction[] table under construction */
	aLookahead      []lookahead_action /* A single new transaction set */
	mnLookahead     int                /* Minimum aLookahead[].lookahead */
	mnAction        int                /* Action associated with mnLookahead */
	mxLookahead     int                /* Maximum aLookahead[].lookahead */
	nLookahead      int                /* Used slots in aLookahead[] */
	nLookaheadAlloc int                /* Slots allocated in aLookahead[] */
	nterminal       int                /* Number of terminal symbols */
	nsymbol         int                /* total number of symbols */
}

/* Return the number of entries in the yy_action table */
func acttab_lookahead_size(x *acttab) int { return x.nAction }

/* The value for the N-th entry in yy_action */
func acttab_yyaction(x *acttab, n int) int { return x.aAction[n].action }

/* The value for the N-th entry in yy_lookahead */
func acttab_yylookahead(x *acttab, n int) int { return x.aAction[n].lookahead }

/* Allocate a new acttab structure */
func acttab_alloc(nsymbol int, nterminal int) *acttab {
	return &acttab{
		nsymbol:   nsymbol,
		nterminal: nterminal,
	}
}

/* Add a new action to the current transaction set.
**
** This routine is called once for each lookahead for a particular
** state.
 */
func acttab_action(p *acttab, lookahead int, action int) {
	if p.nLookahead >= p.nLookaheadAlloc {
		p.nLookaheadAlloc += 25
		p.aLookahead = append(p.aLookahead, make([]lookahead_action, 25)...)
	}
	if p.nLookahead == 0 {
		p.mxLookahead = lookahead
		p.mnLookahead = lookahead
		p.mnAction = action
	} else {
		if p.mxLookahead < lookahead {
			p.mxLookahead = lookahead
		}
		if p.mnLookahead > lookahead {
			p.mnLookahead = lookahead
			p.mnAction = action
		}
	}
	p.aLookahead[p.nLookahead].lookahead = lookahead
	p.aLookahead[p.nLookahead].action = action
	p.nLookahead++
}

/*
** Add the transaction set built up with prior calls to acttab_action()
** into the current action table.  Then reset the transaction set back
** to an empty set in preparation for a new round of acttab_action() calls.
**
** Return the offset into the action table of the new transaction.
**
** If the makeItSafe parameter is true, then the offset is chosen so that
** it is impossible to overread the yy_lookaside[] table regardless of
** the lookaside token.  This is done for the terminal symbols, as they
** come from external inputs and can contain syntax errors.  When makeItSafe
** is false, there is more flexibility in selecting offsets, resulting in
** a smaller table.  For non-terminal symbols, which are never syntax errors,
** makeItSafe can be false.
 */
func acttab_insert(p *acttab, makeItSafe bool) int {
	var i, j, k, n int
	if p.nLookahead <= 0 {
		panic(fmt.Sprintf("Want p.nLookahead > 0; got %d", p.nLookahead))
	}

	/* Make sure we have enough space to hold the expanded action table
	 ** in the worst case.  The worst case occurs if the transaction set
	 ** must be appended to the current action table
	 */
	n = p.nsymbol + 1
	if p.nAction+n >= p.nActionAlloc {
		oldAlloc := p.nActionAlloc
		p.nActionAlloc = p.nAction + n + p.nActionAlloc + 20
		p.aAction = append(p.aAction, make([]lookahead_action, p.nActionAlloc-len(p.aAction))...)
		for i = oldAlloc; i < p.nActionAlloc; i++ {
			p.aAction[i].lookahead = -1
			p.aAction[i].action = -1
		}
	}

	/* Scan the existing action table looking for an offset that is a
	 ** duplicate of the current transaction set.  Fall out of the loop
	 ** if and when the duplicate is found.
	 **
	 ** i is the index in p.aAction[] where p.mnLookahead is inserted.
	 */
	end := 0
	if makeItSafe {
		end = p.mnLookahead
	}
	for i = p.nAction - 1; i >= end; i-- {
		if p.aAction[i].lookahead == p.mnLookahead {
			/* All lookaheads and actions in the aLookahead[] transaction
			 ** must match against the candidate aAction[i] entry. */
			if p.aAction[i].action != p.mnAction {
				continue
			}
			for j = 0; j < p.nLookahead; j++ {
				k = p.aLookahead[j].lookahead - p.mnLookahead + i
				if k < 0 || k >= p.nAction {
					break
				}
				if p.aLookahead[j].lookahead != p.aAction[k].lookahead {
					break
				}
				if p.aLookahead[j].action != p.aAction[k].action {
					break
				}
			}
			if j < p.nLookahead {
				continue
			}

			/* No possible lookahead value that is not in the aLookahead[]
			 ** transaction is allowed to match aAction[i] */
			n = 0
			for j = 0; j < p.nAction; j++ {
				if p.aAction[j].lookahead < 0 {
					continue
				}
				if p.aAction[j].lookahead == j+p.mnLookahead-i {
					n++
				}
			}
			if n == p.nLookahead {
				break /* An exact match is found at offset i */
			}
		}
	}

	/* If no existing offsets exactly match the current transaction, find an
	 ** an empty offset in the aAction[] table in which we can add the
	 ** aLookahead[] transaction.
	 */
	if i < end {
		/* Look for holes in the aAction[] table that fit the current
		 ** aLookahead[] transaction.  Leave i set to the offset of the hole.
		 ** If no holes are found, i is left at p.nAction, which means the
		 ** transaction will be appended. */
		i = 0
		if makeItSafe {
			i = p.mnLookahead
		}
		for ; i < p.nActionAlloc-p.mxLookahead; i++ {
			if p.aAction[i].lookahead < 0 {
				for j = 0; j < p.nLookahead; j++ {
					k = p.aLookahead[j].lookahead - p.mnLookahead + i
					if k < 0 {
						break
					}
					if p.aAction[k].lookahead >= 0 {
						break
					}
				}
				if j < p.nLookahead {
					continue
				}
				for j = 0; j < p.nAction; j++ {
					if p.aAction[j].lookahead == j+p.mnLookahead-i {
						break
					}
				}
				if j == p.nAction {
					break /* Fits in empty slots */
				}
			}
		}
	}
	/* Insert transaction set at index i. */
	if false {
		fmt.Printf("Acttab:")
		for j = 0; j < p.nLookahead; j++ {
			fmt.Printf(" %d", p.aLookahead[j].lookahead)
		}
		fmt.Printf(" inserted at %d\n", i)
	}
	for j = 0; j < p.nLookahead; j++ {
		k = p.aLookahead[j].lookahead - p.mnLookahead + i
		p.aAction[k] = p.aLookahead[j]
		if k >= p.nAction {
			p.nAction = k + 1
		}
	}
	if makeItSafe && i+p.nterminal >= p.nAction {
		p.nAction = i + p.nterminal + 1
	}
	p.nLookahead = 0

	/* Return the offset that is added to the lookahead in order to get the
	 ** index into yy_action of the action */
	return i - p.mnLookahead
}

/*
** Return the size of the action table without the trailing syntax error
** entries.
 */
func acttab_action_size(p *acttab) int {
	n := p.nAction
	for n > 0 && p.aAction[n-1].lookahead < 0 {
		n--
	}
	return n
}

/********************** From the file "build.c" *****************************/
/*
** Routines to construction the finite state machine for the LEMON
** parser generator.
 */

/* Find a precedence symbol of every rule in the grammar.
**
** Those rules which have a precedence symbol coded in the input
** grammar using the "[symbol]" construct will already have the
** rp->precsym field filled.  Other rules take as their precedence
** symbol the first RHS symbol with a defined precedence.  If there
** are not RHS symbols with a defined precedence, the precedence
** symbol field is left blank.
 */
func FindRulePrecedences(xp *lemon) {
	for rp := xp.rule; rp != nil; rp = rp.next {
		if rp.precsym == nil {
			for i := 0; i < len(rp.rhs) && rp.precsym == nil; i++ {
				sp := rp.rhs[i]
				if sp.typ == MULTITERMINAL {
					for j := range sp.subsym {
						if sp.subsym[j].prec >= 0 {
							rp.precsym = sp.subsym[j]
							break
						}
					}
				} else if sp.prec >= 0 {
					rp.precsym = rp.rhs[i]
				}
			}
		}
	}
}

/* Find all nonterminals which will generate the empty string.
** Then go back and compute the first sets of every nonterminal.
** The first set is the set of all terminal symbols which can begin
** a string generated by that nonterminal.
 */
func FindFirstSets(lemp *lemon) {
	for i := 0; i < lemp.nsymbol; i++ {
		lemp.symbols[i].lambda = false
	}
	for i := lemp.nterminal; i < lemp.nsymbol; i++ {
		lemp.symbols[i].firstset = SetNew()
	}

	/* First compute all lambdas */
	for {
		progress := false
		for rp := lemp.rule; rp != nil; rp = rp.next {
			if rp.lhs.lambda {
				continue
			}
			var i int
			for i = 0; i < len(rp.rhs); i++ {
				sp := rp.rhs[i]
				if !(sp.typ == NONTERMINAL || !sp.lambda) {
					panic(fmt.Sprintf("want sp.typ==%d || !sp.lambda; got sp.typ=%d, sp.lambda=%v", NONTERMINAL, sp.typ, sp.lambda))
				}
				if !sp.lambda {
					break
				}
			}
			if i == len(rp.rhs) {
				rp.lhs.lambda = true
				progress = true
			}
		}
		if !progress {
			break
		}
	}

	/* Now compute all first sets */
	for {
		var s1, s2 *symbol
		progress := false
		for rp := lemp.rule; rp != nil; rp = rp.next {
			s1 = rp.lhs
			for i := range rp.rhs {
				s2 = rp.rhs[i]
				if s2.typ == TERMINAL {
					progress = SetAdd(s1.firstset, s2.index) || progress
					break
				} else if s2.typ == MULTITERMINAL {
					for j := range s2.subsym {
						progress = SetAdd(s1.firstset, s2.subsym[j].index) || progress
					}
					break
				} else if s1 == s2 {
					if !s1.lambda {
						break
					}
				} else {
					progress = SetUnion(s1.firstset, s2.firstset) || progress
					if !s2.lambda {
						break
					}
				}
			}
		}
		if !progress {
			break
		}
	}
}

/* Compute all LR(0) states for the grammar.  Links
** are added to between some states so that the LR(1) follow sets
** can be computed later.
 */
func FindStates(lemp *lemon) {
	Configlist_init()

	var sp *symbol
	/* Find the start symbol */
	if lemp.start != "" {
		sp = Symbol_find(lemp.start)
		if sp == nil {
			ErrorMsg(lemp.filename, 0,
				"The specified start symbol \"%s\" is not "+
					"in a nonterminal of the grammar.  \"%s\" will be used as the start "+
					"symbol instead.", lemp.start, lemp.startRule.lhs.name)
			lemp.errorcnt++
			sp = lemp.startRule.lhs
		}
	} else if lemp.startRule != nil {
		sp = lemp.startRule.lhs
	} else {
		ErrorMsg(lemp.filename, 0, "Internal error - no start rule\n")
		os.Exit(1)
	}

	/* Make sure the start symbol doesn't occur on the right-hand side of
	 ** any rule.  Report an error if it does.  (YACC would generate a new
	 ** start symbol in this case.) */
	for rp := lemp.rule; rp != nil; rp = rp.next {
		for i := range rp.rhs {
			if rp.rhs[i] == sp { /* FIX ME:  Deal with multiterminals */
				ErrorMsg(lemp.filename, 0,
					"The start symbol \"%s\" occurs on the "+
						"right-hand side of a rule. This will result in a parser which "+
						"does not work properly.", sp.name)
				lemp.errorcnt++
			}
		}
	}

	/* The basis configuration set for the first state
	 ** is all rules which have the start symbol as their
	 ** left-hand side */
	for rp := sp.rule; rp != nil; rp = rp.nextlhs {
		rp.lhsStart = true
		newcfp := Configlist_addbasis(rp, 0)
		SetAdd(newcfp.fws, 0)
	}

	/* Compute the first state.  All other states will be
	 ** computed automatically during the computation of the first one.
	 ** The returned pointer to the first state is not used. */
	getstate(lemp)
}

/* Return a pointer to a state which is described by the configuration
** list which has been built from calls to Configlist_add.
 */
func getstate(lemp *lemon) *state {
	var stp *state

	/* Extract the sorted basis of the new state.  The basis was constructed
	 ** by prior calls to "Configlist_addbasis()". */
	Configlist_sortbasis()
	bp := Configlist_basis()

	/* Get a state with the same basis */
	stp = State_find(bp)
	if stp != nil {
		/* A state with the same basis already exists!  Copy all the follow-set
		 ** propagation links from the state under construction into the
		 ** preexisting state, then return a pointer to the preexisting state */
		for x, y := bp, stp.bp; x != nil && y != nil; x, y = x.bp, y.bp {
			Plink_copy(&y.bplp, x.bplp)
			Plink_delete(x.fplp)
			x.fplp, x.bplp = nil, nil
		}
		cfp := Configlist_return()
		Configlist_eat(cfp)
	} else {
		/* This really is a new state.  Construct all the details */
		Configlist_closure(lemp)   /* Compute the configuration closure */
		Configlist_sort()          /* Sort the configuration closure */
		cfp := Configlist_return() /* Get a pointer to the config list */
		stp = State_new()          /* A new state structure */

		stp.bp = bp                /* Remember the configuration basis */
		stp.cfp = cfp              /* Remember the configuration closure */
		stp.statenum = lemp.nstate /* Every state gets a sequence number */
		lemp.nstate++
		stp.ap = nil              /* No actions, yet. */
		State_insert(stp, stp.bp) /* Add to the state table */
		buildshifts(lemp, stp)    /* Recursively compute successor states */
	}
	// PrintState(lemp, stp)
	return stp
}

/*
** Return true if two symbols are the same.
 */
func same_symbol(a *symbol, b *symbol) bool {
	if a == b {
		return true
	}
	if a.typ != MULTITERMINAL {
		return false
	}
	if b.typ != MULTITERMINAL {
		return false
	}
	if len(a.subsym) != len(b.subsym) {
		return false
	}
	for i := range a.subsym {
		if a.subsym[i] != b.subsym[i] {
			return false
		}
	}
	return true
}

/* Construct all successor states to the given state.  A "successor"
** state is any state which can be reached by a shift action.
 */
func buildshifts(lemp *lemon, stp *state) {
	var cfp *config    /* For looping thru the config closure of "stp" */
	var bcfp *config   /* For the inner loop on config closure of "stp" */
	var newcfg *config /* */
	var sp *symbol     /* Symbol following the dot in configuration "cfp" */
	var bsp *symbol    /* Symbol following the dot in configuration "bcfp" */

	/* Each configuration becomes complete after it contributes to a successor
	 ** state.  Initially, all configurations are incomplete */
	for cfp = stp.cfp; cfp != nil; cfp = cfp.next {
		cfp.status = INCOMPLETE
	}

	/* Loop through all configurations of the state "stp" */
	for cfp = stp.cfp; cfp != nil; cfp = cfp.next {
		if cfp.status == COMPLETE {
			continue /* Already used by inner loop */
		}
		if cfp.dot >= len(cfp.rp.rhs) {
			continue /* Can't shift this config */
		}
		Configlist_reset()       /* Reset the new config set */
		sp = cfp.rp.rhs[cfp.dot] /* Symbol after the dot */

		/* For every configuration in the state "stp" which has the symbol "sp"
		 ** following its dot, add the same configuration to the basis set under
		 ** construction but with the dot shifted one symbol to the right. */
		for bcfp = cfp; bcfp != nil; bcfp = bcfp.next {
			if bcfp.status == COMPLETE {
				continue /* Already used */
			}
			if bcfp.dot >= len(bcfp.rp.rhs) {
				continue /* Can't shift this one */
			}
			bsp = bcfp.rp.rhs[bcfp.dot] /* Get symbol after dot */
			if !same_symbol(bsp, sp) {
				continue /* Must be same as for "cfp" */
			}
			bcfp.status = COMPLETE /* Mark this config as used */
			newcfg = Configlist_addbasis(bcfp.rp, bcfp.dot+1)
			Plink_add(&newcfg.bplp, bcfp)
		}

		/* Get a pointer to the state described by the basis configuration set
		 ** constructed in the preceding loop */
		newstp := getstate(lemp)

		/* The state "newstp" is reached from the state "stp" by a shift action
		 ** on the symbol "sp" */
		if sp.typ == MULTITERMINAL {
			for i := range sp.subsym {
				// Action_add_debug(1, stp, SHIFT, sp.subsym[i], nil, newstp)
				Action_add(&stp.ap, SHIFT, sp.subsym[i], stateOrRuleUnion{stp: newstp})
			}
		} else {
			// Action_add_debug(2, stp, SHIFT, sp, nil, newstp)
			Action_add(&stp.ap, SHIFT, sp, stateOrRuleUnion{stp: newstp})
		}
	}
}

/*
** Construct the propagation links
 */
func FindLinks(lemp *lemon) {
	/* Housekeeping detail:
	 ** Add to every propagate link a pointer back to the state to
	 ** which the link is attached. */
	for i := 0; i < lemp.nstate; i++ {
		stp := lemp.sorted[i]
		if stp != nil {
			for cfp := stp.cfp; cfp != nil; cfp = cfp.next {
				cfp.stp = stp
			}
		}
	}

	/* Convert all backlinks into forward links.  Only the forward
	 ** links are used in the follow-set computation. */
	for i := 0; i < lemp.nstate; i++ {
		stp := lemp.sorted[i]
		if stp != nil {
			for cfp := stp.cfp; cfp != nil; cfp = cfp.next {
				for plp := cfp.bplp; plp != nil; plp = plp.next {
					other := plp.cfp
					Plink_add(&other.fplp, cfp)
				}
			}
		}
	}
}

/* Compute all followsets.
**
** A followset is the set of all symbols which can come immediately
** after a configuration.
 */
func FindFollowSets(lemp *lemon) {
	for i := 0; i < lemp.nstate; i++ {
		assert(lemp.sorted[i] != nil, "lemp.sorted[i]!=nil")
		for cfp := lemp.sorted[i].cfp; cfp != nil; cfp = cfp.next {
			cfp.status = INCOMPLETE
		}
	}

	for progress := true; progress; {
		progress = false
		for i := 0; i < lemp.nstate; i++ {
			assert(lemp.sorted[i] != nil, "lemp.sorted[i]!=nil")
			for cfp := lemp.sorted[i].cfp; cfp != nil; cfp = cfp.next {
				if cfp.status == COMPLETE {
					continue
				}
				for plp := cfp.fplp; plp != nil; plp = plp.next {
					change := SetUnion(plp.cfp.fws, cfp.fws)
					if change {
						plp.cfp.status = INCOMPLETE
						progress = true
					}
				}
				cfp.status = COMPLETE
			}
		}
	}
}

/* Compute the reduce actions, and resolve conflicts.
 */
func FindActions(lemp *lemon) {
	/* Add all of the reduce actions
	 ** A reduce action is added for each element of the followset of
	 ** a configuration which has its dot at the extreme right.
	 */
	for i := 0; i < lemp.nstate; i++ { /* Loop over all states */
		stp := lemp.sorted[i]
		for cfp := stp.cfp; cfp != nil; cfp = cfp.next { /* Loop over all configurations */
			if len(cfp.rp.rhs) == cfp.dot { /* Is dot at extreme right? */
				for j := 0; j < lemp.nterminal; j++ {
					if SetFind(cfp.fws, j) {
						/* Add a reduce action to the state "stp" which will reduce by the
						 ** rule "cfp.rp" if the lookahead symbol is "lemp.symbols[j]" */
						Action_add(&stp.ap, REDUCE, lemp.symbols[j], stateOrRuleUnion{rp: cfp.rp})
					}
				}
			}
		}
	}

	/* Add the accepting token */
	var sp *symbol
	if lemp.start != "" {
		sp = Symbol_find(lemp.start)
		if sp == nil {
			if lemp.startRule == nil {
				_, _, line, ok := runtime.Caller(0)
				if !ok {
					line = -1
				}
				fmt.Fprintf(os.Stderr, "internal error on source line %d: no start rule\n",
					line)
				os.Exit(1)
			}
			sp = lemp.startRule.lhs
		}
	} else {
		sp = lemp.startRule.lhs
	}
	/* Add to the first state (which is always the starting state of the
	 ** finite state machine) an action to ACCEPT if the lookahead is the
	 ** start nonterminal.  */
	Action_add(&lemp.sorted[0].ap, ACCEPT, sp, stateOrRuleUnion{})

	/* Resolve conflicts */
	for i := 0; i < lemp.nstate; i++ {
		stp := lemp.sorted[i]
		/* assert( stp.ap ); */
		stp.ap = Action_sort(stp.ap)
		for ap := stp.ap; ap != nil && ap.next != nil; ap = ap.next {
			for nap := ap.next; nap != nil && nap.sp == ap.sp; nap = nap.next {
				/* The two actions "ap" and "nap" have the same lookahead.
				 ** Figure out which one should be used */
				lemp.nconflict += resolve_conflict(ap, nap)
			}
		}
	}

	/* Report an error for each rule that can never be reduced. */
	for rp := lemp.rule; rp != nil; rp = rp.next {
		rp.canReduce = false
	}
	for i := 0; i < lemp.nstate; i++ {
		for ap := lemp.sorted[i].ap; ap != nil; ap = ap.next {
			if ap.typ == REDUCE {
				ap.x.rp.canReduce = true
			}
		}
	}
	for rp := lemp.rule; rp != nil; rp = rp.next {
		if rp.canReduce {
			continue
		}
		ErrorMsg(lemp.filename, rp.ruleline, "This rule can not be reduced.\n")
		lemp.errorcnt++
	}
}

/* Resolve a conflict between the two given actions.  If the
** conflict can't be resolved, return non-zero.
**
** NO LONGER TRUE:
**   To resolve a conflict, first look to see if either action
**   is on an error rule.  In that case, take the action which
**   is not associated with the error rule.  If neither or both
**   actions are associated with an error rule, then try to
**   use precedence to resolve the conflict.
**
** If either action is a SHIFT, then it must be apx.  This
** function won't work if apx->type==REDUCE and apy->type==SHIFT.
 */
func resolve_conflict(apx *action, apy *action) int {
	var spx, spy *symbol
	errcnt := 0
	assert(apx.sp == apy.sp, "apx.sp==apy.sp") /* Otherwise there would be no conflict */
	if apx.typ == SHIFT && apy.typ == SHIFT {
		apy.typ = SSCONFLICT
		errcnt++
	}
	if apx.typ == SHIFT && apy.typ == REDUCE {
		spx = apx.sp
		spy = apy.x.rp.precsym
		if spy == nil || spx.prec < 0 || spy.prec < 0 {
			/* Not enough precedence information. */
			apy.typ = SRCONFLICT
			errcnt++
		} else if spx.prec > spy.prec { /* higher precedence wins */
			apy.typ = RD_RESOLVED
		} else if spx.prec < spy.prec {
			apx.typ = SH_RESOLVED
		} else if spx.prec == spy.prec && spx.assoc == RIGHT { /* Use operator */
			apy.typ = RD_RESOLVED /* associativity */
		} else if spx.prec == spy.prec && spx.assoc == LEFT { /* to break tie */
			apx.typ = SH_RESOLVED
		} else {
			assert(spx.prec == spy.prec && spx.assoc == NONE, "spx.prec == spy.prec && spx.assoc == NONE")
			apx.typ = ERROR
		}
	} else if apx.typ == REDUCE && apy.typ == REDUCE {
		spx = apx.x.rp.precsym
		spy = apy.x.rp.precsym
		if spx == nil || spy == nil || spx.prec < 0 ||
			spy.prec < 0 || spx.prec == spy.prec {
			apy.typ = RRCONFLICT
			errcnt++
		} else if spx.prec > spy.prec {
			apy.typ = RD_RESOLVED
		} else if spx.prec < spy.prec {
			apx.typ = RD_RESOLVED
		}
	} else {
		assert(
			(apx.typ == SH_RESOLVED ||
				apx.typ == RD_RESOLVED ||
				apx.typ == SSCONFLICT ||
				apx.typ == SRCONFLICT ||
				apx.typ == RRCONFLICT ||
				apy.typ == SH_RESOLVED ||
				apy.typ == RD_RESOLVED ||
				apy.typ == SSCONFLICT ||
				apy.typ == SRCONFLICT ||
				apy.typ == RRCONFLICT),
			fmt.Sprintf("apx.typ(%d) in {SH_RESOLVED(%d),RD_RESOLVED(%d),SSCONFLICT(%d),SRCONFLICT(%d),RRCONFLICT(%d),SH_RESOLVED(%d),RD_RESOLVED(%d),SSCONFLICT(%d),SRCONFLICT(%d),RRCONFLICT(%d)}",
				apx.typ, SH_RESOLVED, RD_RESOLVED, SSCONFLICT, SRCONFLICT, RRCONFLICT, SH_RESOLVED, RD_RESOLVED, SSCONFLICT, SRCONFLICT, RRCONFLICT))
		/* The REDUCE/SHIFT case cannot happen because SHIFTs come before
		 ** REDUCEs on the list.  If we reach this point it must be because
		 ** the parser conflict had already been resolved. */
	}
	return errcnt
}

/********************* From the file "configlist.c" *************************/
/*
** Routines to processing a configuration list and building a state
** in the LEMON parser generator.
 */

var (
	freelist   *config
	current    *config
	currentend **config
	basis      *config
	basisend   **config
)

/* Return a pointer to a new configuration */
func newconfig() *config {
	return &config{}
}

/* The configuration "old" is no longer used */
func deleteconfig(old *config) {
	old.next = freelist
	freelist = old
}

/* Initialized the configuration list builder */
func Configlist_init() {
	current = nil
	currentend = &current
	basis = nil
	basisend = &basis
	Configtable_init()
}

/* Initialized the configuration list builder */
func Configlist_reset() {
	current = nil
	currentend = &current
	basis = nil
	basisend = &basis
	Configtable_clear()
	return
}

func PrintConfigList() {
	fmt.Printf(" Configlist:")
	for cfp := current; cfp != nil; cfp = cfp.next {
		fmt.Printf(" %d.%d", cfp.rp.iRule, cfp.dot)
	}
	fmt.Printf("\n")

	fmt.Printf(" Configlist_basis: ")
	for cfp := current; cfp != nil; cfp = cfp.bp {
		fmt.Printf(" %d.%d", cfp.rp.iRule, cfp.dot)
	}
	fmt.Printf("\n")
}

/* Add another configuration to the configuration list */
func Configlist_add(rp *rule, dot int) *config {
	var cfp *config
	var model config

	assert(currentend != nil, "currentend!=nil")
	model.rp = rp
	model.dot = dot
	cfp = Configtable_find(&model)
	if cfp == nil {
		cfp = newconfig()
		cfp.rp = rp
		cfp.dot = dot
		cfp.fws = SetNew()
		cfp.stp = nil
		cfp.fplp = nil
		cfp.bplp = nil
		cfp.next = nil
		cfp.bp = nil
		*currentend = cfp
		currentend = &cfp.next
		Configtable_insert(cfp)
	}
	return cfp
}

/* Add a basis configuration to the configuration list */
func Configlist_addbasis(rp *rule, dot int) *config {
	var model config

	assert(basisend != nil, "basisend != nil")
	assert(currentend != nil, "currentend!=nil")
	model.rp = rp
	model.dot = dot
	cfp := Configtable_find(&model)
	if cfp == nil {
		cfp = newconfig()
		cfp.rp = rp
		cfp.dot = dot
		cfp.fws = SetNew()
		cfp.stp = nil
		cfp.fplp, cfp.bplp = nil, nil
		cfp.next = nil
		cfp.bp = nil
		*currentend = cfp
		currentend = &cfp.next
		*basisend = cfp
		basisend = &cfp.bp
		Configtable_insert(cfp)
	}
	return cfp
}

/* Compute the closure of the configuration list */
func Configlist_closure(lemp *lemon) {
	var newcfp *config
	var rp *rule
	var sp *symbol
	var xsp *symbol

	assert(currentend != nil, "currentend!=nil")
	for cfp := current; cfp != nil; cfp = cfp.next {
		rp = cfp.rp
		dot := cfp.dot
		if dot >= len(rp.rhs) {
			continue
		}
		sp = rp.rhs[dot]
		if sp.typ == NONTERMINAL {
			if sp.rule == nil && sp != lemp.errsym {
				ErrorMsg(lemp.filename, rp.line, "Nonterminal \"%s\" has no rules.",
					sp.name)
				lemp.errorcnt++
			}
			for newrp := sp.rule; newrp != nil; newrp = newrp.nextlhs {
				newcfp = Configlist_add(newrp, 0)
				var i int
				for i = dot + 1; i < len(rp.rhs); i++ {
					xsp = rp.rhs[i]
					if xsp.typ == TERMINAL {
						SetAdd(newcfp.fws, xsp.index)
						break
					} else if xsp.typ == MULTITERMINAL {
						for k := range xsp.subsym {
							SetAdd(newcfp.fws, xsp.subsym[k].index)
						}
						break
					} else {
						SetUnion(newcfp.fws, xsp.firstset)
						if !xsp.lambda {
							break
						}
					}
				}
				if i == len(rp.rhs) {
					Plink_add(&cfp.fplp, newcfp)
				}
			}
		}
	}
}

/* Sort the configuration list */
func Configlist_sort() {
	current = msort__config(current)
	currentend = nil
}

/* Sort the basis configuration list */
func Configlist_sortbasis() {
	basis = msort__config_basis(current)
	basisend = nil
}

/* Return a pointer to the head of the configuration list and
** reset the list */
func Configlist_return() *config {
	old := current
	current = nil
	currentend = nil
	return old
}

/* Return a pointer to the head of the configuration list and
** reset the list */
func Configlist_basis() *config {
	var old *config
	old = basis
	basis = nil
	basisend = nil
	return old
}

/* Free all elements of the given configuration list */
func Configlist_eat(cfp *config) {
	var nextcfp *config
	for ; cfp != nil; cfp = nextcfp {
		nextcfp = cfp.next
		assert(cfp.fplp == nil, "cfp.fplp==nil")
		assert(cfp.bplp == nil, "cfp.pblp==nil")
		cfp.fws = nil
		deleteconfig(cfp)
	}
	return
}

/***************** From the file "error.c" *********************************/

/*
** Code for printing error message.
 */
func ErrorMsg(filename string, lineno int, format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s:%d: ", filename, lineno)
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintf(os.Stderr, "\n")
}

/**************** From the file "main.c" ************************************/

/*
** Main program file for the LEMON parser generator.
 */

var azDefine setFlag = make(map[string]bool)

/* Rember the name of the output directory
 */
var outputDir string

var user_templatename string

/* Merge together to lists of rules ordered by rule.iRule */
func Rule_merge(pA *rule, pB *rule) *rule {
	var pFirst *rule
	var ppPrev **rule = &pFirst

	for pA != nil && pB != nil {
		if pA.iRule < pB.iRule {
			*ppPrev = pA
			ppPrev = &pA.next
			pA = pA.next
		} else {
			*ppPrev = pB
			ppPrev = &pB.next
			pB = pB.next
		}
	}
	if pA != nil {
		*ppPrev = pA
	} else {
		*ppPrev = pB
	}
	return pFirst
}

/*
 ** Sort a list of rules in order of increasing iRule value
 */
func Rule_sort(rp *rule) *rule {
	var pNext *rule
	var x [32]*rule
	for rp != nil {
		pNext = rp.next
		rp.next = nil
		var i int
		for i = 0; i < 32-1 && x[i] != nil; i++ {
			rp = Rule_merge(x[i], rp)
			x[i] = nil
		}
		x[i] = rp
		rp = pNext
	}
	rp = nil
	for i := 0; i < 32; i++ {
		rp = Rule_merge(x[i], rp)
	}
	return rp
}

/* Print a single line of the "Parser Stats" output
 */
func stats_line(zLabel string, iValue int) {
	fmt.Printf("  %s%.*s %5d\n", zLabel,
		35-len(zLabel), "................................",
		iValue)
}

func main() {
	var version bool
	var rpflag bool
	var basisflag bool
	var compress bool
	var quiet bool
	var statistics bool
	var nolinenosflag bool
	var noResort bool
	var sqlFlag bool
	var printPP bool

	flag.BoolVar(&basisflag, "b", false, "Print only the basis in report.")
	flag.BoolVar(&compress, "c", false, "Don't compress the action table.")
	flag.StringVar(&outputDir, "d", "", "Output directory.  Default '.'")
	flag.Var(&azDefine, "D", "Define an %ifdef macro.")
	flag.BoolVar(&printPP, "E", false, "Print input file after preprocessing.")
	_ = flag.String("f", "", "Ignored.  (Placeholder for -f compiler options.)")
	flag.BoolVar(&rpflag, "g", false, "Print grammar without actions.")
	_ = flag.String("I", "", "Ignored.  (Placeholder for -I compiler options.)")
	flag.BoolVar(&nolinenosflag, "l", false, "Do not print #line statements.")
	_ = flag.String("O", "", "Ignored.  (Placeholder for -O compiler options.)")
	flag.BoolVar(&showPrecedenceConflict, "p", false, "Show conflicts resolved by precedence rules")
	flag.BoolVar(&quiet, "q", false, "(Quiet) Don't print the report file.")
	flag.BoolVar(&noResort, "r", false, "Do not sort or renumber states")
	flag.BoolVar(&statistics, "s", false, "Print parser stats to standard output.")
	flag.BoolVar(&sqlFlag, "S", false, "Generate the *.sql file describing the parser tables.")
	flag.BoolVar(&version, "x", false, "Print the version number.")
	flag.StringVar(&user_templatename, "T", "", "Specify a template file.")
	_ = flag.String("W", "", "Ignored.  (Placeholder for -W compiler options.)")

	flag.Parse()

	var lem lemon
	var rp *rule

	if version {
		fmt.Printf("Lemon version 1.0\n")
		os.Exit(0)
	}
	if len(flag.Args()) != 1 {
		fmt.Fprintf(os.Stderr, "Exactly one filename argument is required.\n")
		os.Exit(1)
	}
	lem.errorcnt = 0

	/* Initialize the machine */
	// Strsafe_init()
	Symbol_init()
	State_init()
	lem.argv0 = os.Args[0]
	lem.filename = flag.Args()[0]
	lem.basisflag = basisflag
	lem.nolinenosflag = nolinenosflag
	lem.printPreprocessed = printPP
	Symbol_new("$")

	/* Parse the input file */
	Parse(&lem)
	if lem.printPreprocessed || lem.errorcnt > 0 {
		os.Exit(lem.errorcnt)
	}
	if lem.nrule == 0 {
		fmt.Fprintf(os.Stderr, "Empty grammar.\n")
		os.Exit(1)
	}
	lem.errsym = Symbol_find("error")

	/* Count and index the symbols of the grammar */
	Symbol_new("{default}")
	lem.nsymbol = Symbol_count()
	lem.symbols = Symbol_arrayof()
	for i := 0; i < lem.nsymbol; i++ {
		lem.symbols[i].index = i
	}
	sort.Sort(symbolSorter(lem.symbols[:lem.nsymbol]))
	var i int
	for i = 0; i < lem.nsymbol; i++ {
		lem.symbols[i].index = i
	}
	for lem.symbols[i-1].typ == MULTITERMINAL {
		i--
	}
	assert(lem.symbols[i-1].name == "{default}", `lem.symbols[i-1].name == "{default}"`)
	lem.nsymbol = i - 1

	for i = 1; firstRuneIsUpper(lem.symbols[i].name); i++ {
	}
	lem.nterminal = i
	/* Assign sequential rule numbers.  Start with 0.  Put rules that have no
	 ** reduce action C-code associated with them last, so that the switch()
	 ** statement that selects reduction actions will have a smaller jump table.
	 */

	for i, rp = 0, lem.rule; rp != nil; rp = rp.next {
		if rp.code != "" {
			rp.iRule = i
			i++
		} else {
			rp.iRule = -1
		}
	}
	lem.nruleWithAction = i
	for rp := lem.rule; rp != nil; rp = rp.next {
		if rp.iRule < 0 {
			rp.iRule = i
			i++
		}
	}
	lem.startRule = lem.rule
	lem.rule = Rule_sort(lem.rule)

	/* Generate a reprint of the grammar, if requested on the command line */
	if rpflag {
		Reprint(&lem)
	} else {
		/* Initialize the size for all follow and first sets */
		// SetSize(lem.nterminal + 1)

		/* Find the precedence for every production rule (that has one) */
		FindRulePrecedences(&lem)

		/* Compute the lambda-nonterminals and the first-sets for every
		 ** nonterminal */
		FindFirstSets(&lem)

		/* Compute all LR(0) states.  Also record follow-set propagation
		 ** links so that the follow-set can be computed later */
		lem.nstate = 0
		FindStates(&lem)
		lem.sorted = State_arrayof()
		// PrintLemon(&lem)

		/* Tie up loose ends on the propagation links */
		FindLinks(&lem)

		/* Compute the follow set of every reducible configuration */
		FindFollowSets(&lem)

		/* Compute the action tables */
		FindActions(&lem)

		/* Compress the action tables */
		if !compress {
			CompressTables(&lem)
		}

		/* Reorder and renumber the states so that states with fewer choices
		 ** occur at the end.  This is an optimization that helps make the
		 ** generated parser tables smaller. */
		if !noResort {
			ResortStates(&lem)
		}

		/* Generate a report of the parser generated.  (the "y.output" file) */
		if !quiet {
			ReportOutput(&lem)
		}

		/* Generate the source code for the parser */
		ReportTable(&lem, sqlFlag)
	}
	if statistics {
		fmt.Printf("Parser statistics:\n")
		stats_line("terminal symbols", lem.nterminal)
		stats_line("non-terminal symbols", lem.nsymbol-lem.nterminal)
		stats_line("total symbols", lem.nsymbol)
		stats_line("rules", lem.nrule)
		stats_line("states", lem.nxstate)
		stats_line("conflicts", lem.nconflict)
		stats_line("action table entries", lem.nactiontab)
		stats_line("lookahead table entries", lem.nlookaheadtab)
		stats_line("total table size (bytes)", lem.tablesize)
	}
	if lem.nconflict > 0 {
		fmt.Fprintf(os.Stderr, "%d parsing conflicts.\n", lem.nconflict)
	}

	/* return 0 on success, 1 on failure. */
	if lem.errorcnt > 0 || lem.nconflict > 0 {
		os.Exit(1)
	}
}

/******************** From the file "msort.c" *******************************/
/*
** A generic merge-sort program.
**
** USAGE:
** Let "ptr" be a pointer to some structure which is at the head of
** a null-terminated list.  Then to sort the list call:
**
**     ptr = msort(ptr,&(ptr->next),cmpfnc);
**
** In the above, "cmpfnc" is a pointer to a function which compares
** two instances of the structure and returns an integer, as in
** strcmp.  The second argument is a pointer to the pointer to the
** second element of the linked list.  This address is used to compute
** the offset to the "next" field within the structure.  The offset to
** the "next" field must be constant for all structures in the list.
**
** The function returns a new pointer which is the head of the list
** after sorting.
**
** ALGORITHM:
** Merge-sort.
 */

/*
** Inputs:
**   a:       A sorted, null-terminated linked list.  (May be null).
**   b:       A sorted, null-terminated linked list.  (May be null).
**   cmp:     A pointer to the comparison function.
**   offset:  Offset in the structure to the "next" field.
**
** Return Value:
**   A pointer to the head of a sorted list containing the elements
**   of both a and b.
**
** Side effects:
**   The "next" pointers for elements in the lists a and b are
**   changed.
 */

/// We split merge into its three uses. Generics will come in handy here.

func merge__action(a *action, b *action) *action {

	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	var ptr, head *action

	if actioncmp(a, b) <= 0 {
		ptr = a
		a = a.next
	} else {
		ptr = b
		b = b.next
	}

	head = ptr

	for a != nil && b != nil {
		if actioncmp(a, b) <= 0 {
			ptr.next = a
			ptr = a
			a = a.next
		} else {
			ptr.next = b
			ptr = b
			b = b.next
		}
	}

	if a != nil {
		ptr.next = a
	} else {
		ptr.next = b
	}

	return head
}

func merge__config(a *config, b *config) *config {

	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	var ptr, head *config

	if Configcmp(a, b) <= 0 {
		ptr = a
		a = a.next
	} else {
		ptr = b
		b = b.next
	}

	head = ptr

	for a != nil && b != nil {
		if Configcmp(a, b) <= 0 {
			ptr.next = a
			ptr = a
			a = a.next
		} else {
			ptr.next = b
			ptr = b
			b = b.next
		}
	}

	if a != nil {
		ptr.next = a
	} else {
		ptr.next = b
	}

	return head
}

func merge__config_basis(a *config, b *config) *config {

	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	var ptr, head *config

	if Configcmp(a, b) <= 0 {
		ptr = a
		a = a.bp
	} else {
		ptr = b
		b = b.bp
	}

	head = ptr

	for a != nil && b != nil {
		if Configcmp(a, b) <= 0 {
			ptr.bp = a
			ptr = a
			a = a.bp
		} else {
			ptr.bp = b
			ptr = b
			b = b.bp
		}
	}

	if a != nil {
		ptr.bp = a
	} else {
		ptr.bp = b
	}

	return head
}

/*
** Inputs:
**   list:      Pointer to a singly-linked list of structures.
**   next:      Pointer to pointer to the second element of the list.
**   cmp:       A comparison function.
**
** Return Value:
**   A pointer to the head of a sorted list containing the elements
**   originally in list.
**
** Side effects:
**   The "next" pointers for elements in list are changed.
 */

const LISTSIZE = 30

/// We split msort into its three uses. Generics will come in handy here.

func msort__action(list *action) *action {
	var ep *action
	set := make([]*action, LISTSIZE)
	for list != nil {
		ep = list
		list = list.next
		ep.next = nil

		i := 0
		for ; i < LISTSIZE-1 && set[i] != nil; i++ {
			ep = merge__action(ep, set[i])
			set[i] = nil
		}
		set[i] = ep
	}
	ep = nil
	i := 0
	for ; i < LISTSIZE; i++ {
		if set[i] != nil {
			ep = merge__action(set[i], ep)
		}
	}
	return ep
}

func msort__config(list *config) *config {
	var ep *config
	set := make([]*config, LISTSIZE)
	for list != nil {
		ep = list
		list = list.next
		ep.next = nil

		i := 0
		for ; i < LISTSIZE-1 && set[i] != nil; i++ {
			ep = merge__config(ep, set[i])
			set[i] = nil
		}
		set[i] = ep
	}
	ep = nil
	i := 0
	for ; i < LISTSIZE; i++ {
		if set[i] != nil {
			ep = merge__config(set[i], ep)
		}
	}
	return ep
}

func msort__config_basis(list *config) *config {
	var ep *config
	set := make([]*config, LISTSIZE)
	for list != nil {
		ep = list
		list = list.bp
		ep.bp = nil

		i := 0
		for ; i < LISTSIZE-1 && set[i] != nil; i++ {
			ep = merge__config_basis(ep, set[i])
			set[i] = nil
		}
		set[i] = ep
	}
	ep = nil
	i := 0
	for ; i < LISTSIZE; i++ {
		if set[i] != nil {
			ep = merge__config_basis(set[i], ep)
		}
	}
	return ep
}

/*********************** From the file "parse.c" ****************************/
/*
** Input file parser for the LEMON parser generator.
 */

/* The state of the parser */
type e_state int

const (
	INITIALIZE e_state = iota
	WAITING_FOR_DECL_OR_RULE
	WAITING_FOR_DECL_KEYWORD
	WAITING_FOR_DECL_ARG
	WAITING_FOR_PRECEDENCE_SYMBOL
	WAITING_FOR_ARROW
	IN_RHS
	LHS_ALIAS_1
	LHS_ALIAS_2
	LHS_ALIAS_3
	RHS_ALIAS_1
	RHS_ALIAS_2
	PRECEDENCE_MARK_1
	PRECEDENCE_MARK_2
	RESYNC_AFTER_RULE_ERROR
	RESYNC_AFTER_DECL_ERROR
	WAITING_FOR_DESTRUCTOR_SYMBOL
	WAITING_FOR_DATATYPE_SYMBOL
	WAITING_FOR_FALLBACK_ID
	WAITING_FOR_WILDCARD_ID
	WAITING_FOR_CLASS_ID
	WAITING_FOR_CLASS_TOKEN
	WAITING_FOR_TOKEN_NAME
)

type pstate struct {
	filename        string    /* Name of the input file */
	tokenlineno     int       /* Linenumber at which current token starts */
	errorcnt        int       /* Number of errors so far */
	tokenstart      int       /* T̵e̵x̵t̵ start position of current token */
	gp              *lemon    /* Global state vector */
	state           e_state   /* The state of the parser */
	fallback        *symbol   /* The fallback token */
	tkclass         *symbol   /* Token class symbol */
	lhs             *symbol   /* Left-hand side of current rule */
	lhsalias        string    /* Alias for the LHS */
	nrhs            int       /* Number of right-hand side symbols seen */
	rhs             []*symbol /* RHS symbols */
	alias           []string  /* Aliases for each RHS symbol (or NULL) */
	prevrule        *rule     /* Previous rule parsed */
	declkeyword     string    /* Keyword of a declaration */
	declargslot     *string   /* Where the declaration argument should be put */
	insertLineMacro bool      /* Add #line before declaration insert */
	decllinenoslot  *int      /* Where to write declaration line number */
	declassoc       e_assoc   /* Assign this association to decl arguments */
	preccounter     int       /* Assign this precedence to decl arguments */
	firstrule       *rule     /* Pointer to first rule in the grammar */
	lastrule        *rule     /* Pointer to the most recently parsed rule */
}

/* Parse a single token */
func parseonetoken(psp *pstate, runes []rune) {
	x := string(runes)
	x0 := runes[0]
	var x1, x2 rune
	if len(runes) > 1 {
		x1 = runes[1]
		if len(runes) > 2 {
			x2 = runes[2]
		}
	}

	if false { // #if 0
		fmt.Printf("%s:%d: Token=[%s] state=%d\n", psp.filename, psp.tokenlineno, x, psp.state)
	} // #endif

	switch psp.state {
	case INITIALIZE:
		psp.prevrule = nil
		psp.preccounter = 0
		psp.firstrule = nil
		psp.lastrule = nil
		psp.gp.nrule = 0
		/* fall through */
		fallthrough
	case WAITING_FOR_DECL_OR_RULE:
		if x0 == '%' {
			psp.state = WAITING_FOR_DECL_KEYWORD
		} else if islower(x0) {
			psp.lhs = Symbol_new(x)
			psp.nrhs = 0
			psp.rhs = psp.rhs[:0]
			psp.alias = psp.alias[:0]
			psp.lhsalias = ""
			psp.state = WAITING_FOR_ARROW
		} else if x0 == '{' {
			if psp.prevrule == nil {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"There is no prior rule upon which to attach the code fragment which begins on this line.")
				psp.errorcnt++
			} else if psp.prevrule.code != "" {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"Code fragment beginning on this line is not the first to follow the previous rule.")
				psp.errorcnt++
			} else if x == "{NEVER-REDUCE" {
				psp.prevrule.neverReduce = true
			} else {
				psp.prevrule.line = psp.tokenlineno
				psp.prevrule.code = string(runes[1:])
				psp.prevrule.noCode = false
			}
		} else if x0 == '[' {
			psp.state = PRECEDENCE_MARK_1
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Token \"%s\" should be either \"%%\" or a nonterminal name.",
				x)
			psp.errorcnt++
		}

	case PRECEDENCE_MARK_1:
		if !unicode.IsUpper(x0) {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"The precedence symbol must be a terminal.")
			psp.errorcnt++
		} else if psp.prevrule == nil {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"There is no prior rule to assign precedence \"[%s]\".", x)
			psp.errorcnt++
		} else if psp.prevrule.precsym != nil {
			ErrorMsg(psp.filename, psp.tokenlineno, "Precedence mark on this line is not the first to follow the previous rule.")
			psp.errorcnt++
		} else {
			psp.prevrule.precsym = Symbol_new(x)
		}
		psp.state = PRECEDENCE_MARK_2

	case PRECEDENCE_MARK_2:
		if x0 != ']' {
			ErrorMsg(psp.filename, psp.tokenlineno, "Missing \"]\" on precedence mark.")
			psp.errorcnt++
		}
		psp.state = WAITING_FOR_DECL_OR_RULE

	case WAITING_FOR_ARROW:
		if x0 == ':' && x1 == ':' && x2 == '=' {
			psp.state = IN_RHS
		} else if x0 == '(' {
			psp.state = LHS_ALIAS_1
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Expected to see a \":\" following the LHS symbol \"%s\"; got %s%s%s.",
				psp.lhs.name, string(x0), string(x1), string(x2))
			psp.errorcnt++
			psp.state = RESYNC_AFTER_RULE_ERROR
		}

	case LHS_ALIAS_1:
		if unicode.IsLetter(x0) {
			psp.lhsalias = x
			psp.state = LHS_ALIAS_2
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"\"%s\" is not a valid alias for the LHS \"%s\"\n",
				x, psp.lhs.name)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_RULE_ERROR
		}

	case LHS_ALIAS_2:
		if x0 == ')' {
			psp.state = LHS_ALIAS_3
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Missing \")\" following LHS alias name \"%s\".", psp.lhsalias)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_RULE_ERROR
		}

	case LHS_ALIAS_3:
		if x0 == ':' && x1 == ':' && x2 == '=' {
			psp.state = IN_RHS
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Missing \".\" following: \"%s(%s)\".",
				psp.lhs.name, psp.lhsalias)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_RULE_ERROR
		}

	case IN_RHS:
		if x0 == '.' {
			rp := &rule{
				ruleline: psp.tokenlineno,
				lhs:      psp.lhs,

				lhsalias: psp.lhsalias,
				code:     "",
				noCode:   true,
				precsym:  nil,
				index:    psp.gp.nrule,
				nextlhs:  psp.lhs.rule,
				next:     nil,
			}
			psp.gp.nrule += 1
			rp.rhs = make([]*symbol, psp.nrhs)
			copy(rp.rhs, psp.rhs)
			rp.rhsalias = make([]string, psp.nrhs)
			copy(rp.rhsalias, psp.alias)
			for i, rhs := range rp.rhs {
				if rp.rhsalias[i] != "" {
					rhs.bContent = true
				}
			}
			rp.lhs.rule = rp

			if psp.firstrule == nil {
				psp.firstrule = rp
				psp.lastrule = rp
			} else {
				psp.lastrule.next = rp
				psp.lastrule = rp
			}
			psp.prevrule = rp
			psp.state = WAITING_FOR_DECL_OR_RULE
		} else if unicode.IsLetter(x0) {
			if len(psp.rhs) >= MAXRHS {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"Too many symbols on RHS of rule beginning at \"%s\".",
					x)
				psp.errorcnt++
				psp.state = RESYNC_AFTER_RULE_ERROR
			} else {
				psp.rhs = append(psp.rhs, Symbol_new(x))
				psp.alias = append(psp.alias, "")
				psp.nrhs++
				if len(psp.rhs) != psp.nrhs || len(psp.alias) != psp.nrhs {
					msg := fmt.Sprintf("BANG! nrhs=%d, len(rhs)=%d, len(alias)=%d", psp.nrhs, len(psp.rhs), len(psp.alias))
					panic(msg)
				}
			}
		} else if (x0 == '|' || x0 == '/') && psp.nrhs > 0 && unicode.IsUpper(x1) {
			msp := psp.rhs[psp.nrhs-1]
			if msp.typ != MULTITERMINAL {
				origsp := msp
				msp = &symbol{
					typ:    MULTITERMINAL,
					subsym: []*symbol{origsp},
					name:   origsp.name,
				}
				psp.rhs[psp.nrhs-1] = msp
			}
			msp.subsym = append(msp.subsym, Symbol_new(string(runes[1:])))
			if islower(x1) || msp.subsym[0].name != "" && islower([]rune(msp.subsym[0].name)[0]) {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"Cannot form a compound containing a non-terminal")
				psp.errorcnt++
			}
		} else if x0 == '(' && len(psp.rhs) > 0 {
			psp.state = RHS_ALIAS_1
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Illegal character on RHS of rule: \"%s\".", x)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_RULE_ERROR
		}

	case RHS_ALIAS_1:
		if unicode.IsLetter(x0) {
			psp.alias[psp.nrhs-1] = x
			psp.state = RHS_ALIAS_2
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"\"%s\" is not a valid alias for the RHS symbol \"%s\"\n",
				x, psp.rhs[psp.nrhs-1].name)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_RULE_ERROR
		}

	case RHS_ALIAS_2:
		if x0 == ')' {
			psp.state = IN_RHS
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Missing \")\" following LHS alias name \"%s\".", psp.lhsalias)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_RULE_ERROR
		}

	case WAITING_FOR_DECL_KEYWORD:
		if unicode.IsLetter(x0) {
			psp.declkeyword = x
			psp.declargslot = nil
			psp.decllinenoslot = nil
			psp.insertLineMacro = true
			psp.state = WAITING_FOR_DECL_ARG
			if x == "name" {
				psp.declargslot = &(psp.gp.name)
				psp.insertLineMacro = false
			} else if x == "include" {
				psp.declargslot = &(psp.gp.include)
			} else if x == "code" {
				psp.declargslot = &(psp.gp.extracode)
			} else if x == "token_destructor" {
				psp.declargslot = &psp.gp.tokendest
			} else if x == "default_destructor" {
				psp.declargslot = &psp.gp.vardest
			} else if x == "token_prefix" {
				psp.declargslot = &psp.gp.tokenprefix
				psp.insertLineMacro = false
			} else if x == "syntax_error" {
				psp.declargslot = &(psp.gp.error)
			} else if x == "parse_accept" {
				psp.declargslot = &(psp.gp.accept)
			} else if x == "parse_failure" {
				psp.declargslot = &(psp.gp.failure)
			} else if x == "stack_overflow" {
				psp.declargslot = &(psp.gp.overflow)
			} else if x == "extra_argument" {
				psp.declargslot = &(psp.gp.arg)
				psp.insertLineMacro = false
			} else if x == "extra_context" {
				psp.declargslot = &(psp.gp.ctx)
				psp.insertLineMacro = false
			} else if x == "token_type" {
				psp.declargslot = &(psp.gp.tokentype)
				psp.insertLineMacro = false
			} else if x == "default_type" {
				psp.declargslot = &(psp.gp.vartype)
				psp.insertLineMacro = false
			} else if x == "stack_size" {
				psp.declargslot = &(psp.gp.stacksize)
				psp.insertLineMacro = false
			} else if x == "start_symbol" {
				psp.declargslot = &(psp.gp.start)
				psp.insertLineMacro = false
			} else if x == "left" {
				psp.preccounter++
				psp.declassoc = LEFT
				psp.state = WAITING_FOR_PRECEDENCE_SYMBOL
			} else if x == "right" {
				psp.preccounter++
				psp.declassoc = RIGHT
				psp.state = WAITING_FOR_PRECEDENCE_SYMBOL
			} else if x == "nonassoc" {
				psp.preccounter++
				psp.declassoc = NONE
				psp.state = WAITING_FOR_PRECEDENCE_SYMBOL
			} else if x == "destructor" {
				psp.state = WAITING_FOR_DESTRUCTOR_SYMBOL
			} else if x == "type" {
				psp.state = WAITING_FOR_DATATYPE_SYMBOL
			} else if x == "fallback" {
				psp.fallback = nil
				psp.state = WAITING_FOR_FALLBACK_ID
			} else if x == "token" {
				psp.state = WAITING_FOR_TOKEN_NAME
			} else if x == "wildcard" {
				psp.state = WAITING_FOR_WILDCARD_ID
			} else if x == "token_class" {
				psp.state = WAITING_FOR_CLASS_ID
			} else {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"Unknown declaration keyword: \"%%%s\".", x)
				psp.errorcnt++
				psp.state = RESYNC_AFTER_DECL_ERROR
			}
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Illegal declaration keyword: \"%s\".", x)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_DECL_ERROR
		}

	case WAITING_FOR_DESTRUCTOR_SYMBOL:
		if !unicode.IsLetter(x0) {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Symbol name missing after %%destructor keyword")
			psp.errorcnt++
			psp.state = RESYNC_AFTER_DECL_ERROR
		} else {
			sp := Symbol_new(x)
			psp.declargslot = &sp.destructor
			psp.decllinenoslot = &sp.destLineno
			psp.insertLineMacro = true
			psp.state = WAITING_FOR_DECL_ARG
		}

	case WAITING_FOR_DATATYPE_SYMBOL:
		if !unicode.IsLetter(x0) {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Symbol name missing after %%type keyword")
			psp.errorcnt++
			psp.state = RESYNC_AFTER_DECL_ERROR
		} else {
			sp := Symbol_find(x)
			if sp != nil && sp.datatype != "" {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"Symbol %%type \"%s\" already defined", x)
				psp.errorcnt++
				psp.state = RESYNC_AFTER_DECL_ERROR
			} else {
				if sp == nil {
					sp = Symbol_new(x)
				}
				psp.declargslot = &sp.datatype
				psp.insertLineMacro = false
				psp.state = WAITING_FOR_DECL_ARG
			}
		}

	case WAITING_FOR_PRECEDENCE_SYMBOL:
		if x0 == '.' {
			psp.state = WAITING_FOR_DECL_OR_RULE
		} else if unicode.IsUpper(x0) {
			sp := Symbol_new(x)
			if sp.prec >= 0 {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"Symbol \"%s\" has already be given a precedence.", x)
				psp.errorcnt++
			} else {
				sp.prec = psp.preccounter
				sp.assoc = psp.declassoc
			}
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Can't assign a precedence to \"%s\".", x)
			psp.errorcnt++
		}

	case WAITING_FOR_DECL_ARG:
		if x0 == '{' || x0 == '"' || isalnum(x0) {
			zNew := x
			if zNew[0] == '"' || zNew[0] == '{' {
				zNew = string(runes[1:])
			}

			addLineMacro := !psp.gp.nolinenosflag && psp.insertLineMacro && psp.tokenlineno > 1 && (psp.decllinenoslot == nil || *psp.decllinenoslot != 0)
			if addLineMacro {
				zLine := fmt.Sprintf("//line %d ", psp.tokenlineno)

				if *psp.declargslot != "" && !strings.HasSuffix(*psp.declargslot, "\n") {
					*psp.declargslot += "\n"
				}
				*psp.declargslot += zLine
				*psp.declargslot += "\""
				*psp.declargslot += strings.ReplaceAll(psp.filename, "\\", "\\\\")
				*psp.declargslot += "\"\n"

			}
			if psp.decllinenoslot != nil && *psp.decllinenoslot == 0 {
				*psp.decllinenoslot = psp.tokenlineno
			}
			*psp.declargslot += zNew
			psp.state = WAITING_FOR_DECL_OR_RULE
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Illegal argument to %%%s: %s", psp.declkeyword, x)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_DECL_ERROR
		}

	case WAITING_FOR_FALLBACK_ID:
		if x0 == '.' {
			psp.state = WAITING_FOR_DECL_OR_RULE
		} else if !unicode.IsUpper(x0) {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"%%fallback argument \"%s\" should be a token", x)
			psp.errorcnt++
		} else {
			sp := Symbol_new(x)
			if psp.fallback == nil {
				psp.fallback = sp
			} else if sp.fallback != nil {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"More than one fallback assigned to token %s", x)
				psp.errorcnt++
			} else {
				sp.fallback = psp.fallback
				psp.gp.has_fallback = true
			}
		}

	case WAITING_FOR_TOKEN_NAME:
		/* Tokens do not have to be declared before use.  But they can be
		 ** in order to control their assigned integer number.  The number for
		 ** each token is assigned when it is first seen.  So by including
		 **
		 **     %token ONE TWO THREE.
		 **
		 ** early in the grammar file, that assigns small consecutive values
		 ** to each of the tokens ONE TWO and THREE.
		 */
		if x0 == '.' {
			psp.state = WAITING_FOR_DECL_OR_RULE
		} else if !unicode.IsUpper(x0) {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"%%token argument \"%s\" should be a token", x)
			psp.errorcnt++
		} else {
			_ = Symbol_new(x)
		}

	case WAITING_FOR_WILDCARD_ID:
		if x0 == '.' {
			psp.state = WAITING_FOR_DECL_OR_RULE
		} else if !unicode.IsUpper(x0) {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"%%wildcard argument \"%s\" should be a token", x)
			psp.errorcnt++
		} else {
			sp := Symbol_new(x)
			if psp.gp.wildcard == nil {
				psp.gp.wildcard = sp
			} else {
				ErrorMsg(psp.filename, psp.tokenlineno,
					"Extra wildcard to token: %s", x)
				psp.errorcnt++
			}
		}

	case WAITING_FOR_CLASS_ID:
		if !islower(x0) {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"%%token_class must be followed by an identifier: %s", x)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_DECL_ERROR
		} else if Symbol_find(x) != nil {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"Symbol \"%s\" already used", x)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_DECL_ERROR
		} else {
			psp.tkclass = Symbol_new(x)
			psp.tkclass.typ = MULTITERMINAL
			psp.state = WAITING_FOR_CLASS_TOKEN
		}

	case WAITING_FOR_CLASS_TOKEN:
		if x0 == '.' {
			psp.state = WAITING_FOR_DECL_OR_RULE
		} else if unicode.IsUpper(x0) || ((x0 == '|' || x0 == '/') && unicode.IsUpper(x1)) {
			msp := psp.tkclass
			if !unicode.IsUpper(x0) {
				x = string(runes[1:])
			}
			msp.subsym = append(msp.subsym, Symbol_new(x))
		} else {
			ErrorMsg(psp.filename, psp.tokenlineno,
				"%%token_class argument \"%s\" should be a token", x)
			psp.errorcnt++
			psp.state = RESYNC_AFTER_DECL_ERROR
		}

	case RESYNC_AFTER_RULE_ERROR:
		/*   //    if( x0=='.' ) {psp.state = WAITING_FOR_DECL_OR_RULE;}
		 **  //    break; */
	case RESYNC_AFTER_DECL_ERROR:
		if x0 == '.' {
			psp.state = WAITING_FOR_DECL_OR_RULE
		} else if x0 == '%' {
			psp.state = WAITING_FOR_DECL_KEYWORD
		}
	}
}

/* The text in the input is part of the argument to an %ifdef or %ifndef.
** Evaluate the text as a boolean expression.  Return true or false.
 */
func eval_preprocessor_boolean(z []rune, lineno int) int {
	neg := false
	res := 0
	var i int
	var zi rune
	okTerm := true

	for i := 0; i < len(z); i++ {
		zi = z[i]
		var zi1 rune
		if i+1 < len(z) {
			zi1 = z[i+1]
		}
		if unicode.IsSpace(zi) {
			continue
		}
		if zi == '!' {
			if !okTerm {
				goto pp_syntax_error
			}
			neg = !neg
			continue
		}
		if zi == '|' && zi1 == '|' {
			if okTerm {
				goto pp_syntax_error
			}
			if res != 0 {
				return 1
			}
			i++
			okTerm = true
			continue
		}
		if zi == '&' && zi1 == '&' {
			if okTerm {
				goto pp_syntax_error
			}

			if res == 0 {
				return 0
			}
			i++
			okTerm = true
			continue
		}
		if zi == '(' {
			n := 1
			if !okTerm {
				goto pp_syntax_error
			}

			for k := i + 1; k < len(z); k++ {
				if z[k] == ')' {
					n--
					if n == 0 {
						res = eval_preprocessor_boolean(z[i+1:k], -1)
						if res < 0 {
							i = i - res
							goto pp_syntax_error
						}
						i = k
						break
					}
				} else if z[k] == '(' {
					n++
				} else if z[k] == 0 {
					i = k
					goto pp_syntax_error
				}
			}
			if neg {
				if res != 0 {
					res = 0
				} else {
					res = 1
				}
				neg = false
			}
			okTerm = false
			continue
		}
		if unicode.IsLetter(zi) {
			if !okTerm {
				goto pp_syntax_error
			}

			var k int
			for k = i + 1; k < len(z) && (isalnum(z[k]) || z[k] == '_'); k++ {
			}
			n := k - i
			res = 0
			if azDefine[string(z[i:i+n])] {
				res = 1
			}
			i = k - 1
			if neg {
				if res != 0 {
					res = 0
				} else {
					res = 1
				}
				neg = false
			}
			okTerm = false
			continue
		}
		goto pp_syntax_error
	}
	return res

pp_syntax_error:
	if lineno > 0 {
		fmt.Fprintf(os.Stderr, "%%if syntax error on line %d.\n", lineno)
		fmt.Fprintf(os.Stderr, "  %.*s <-- syntax error here\n", i+1, string(z))
		os.Exit(1)
	}
	return -(i + 1)
}

/* Run the preprocessor over the input file text.  The global variables
** azDefine[0] through azDefine[nDefine-1] contains the names of all defined
** macros.  This routine looks for "%ifdef" and "%ifndef" and "%endif" and
** comments them out.  Text in between is also commented out as appropriate.
 */
func preprocess_input(z []rune) {
	var j int
	exclude := 0
	start := 0
	lineno := 1
	start_lineno := 1
	for i := range z {
		if z[i] == '\n' {
			lineno++
		}
		if z[i] != '%' || (i > 0 && z[i-1] != '\n') {
			continue
		}
		if len(z) >= i+6 && string(z[i:i+6]) == "%endif" && (len(z) == i+6 || unicode.IsSpace(z[i+6])) {
			if exclude != 0 {
				exclude--
				if exclude == 0 {
					for j = start; j < i; j++ {
						if z[j] != '\n' {
							z[j] = ' '
						}
					}
				}
			}
			for j = i; j < len(z) && z[j] != '\n'; j++ {
				z[j] = ' '
			}
		} else if len(z) >= i+6 && string(z[i:i+5]) == "%else" && unicode.IsSpace(z[i+5]) {
			if exclude == 1 {
				exclude = 0
				for j = start; j < i; j++ {
					if z[j] != '\n' {
						z[j] = ' '
					}
				}
			} else if exclude == 0 {
				exclude = 1
				start = i
				start_lineno = lineno
			}
			for j = i; j < len(z) && z[j] != '\n'; j++ {
				z[j] = ' '
			}
		} else if (len(z) >= i+7 && string(z[i:i+7]) == "%ifdef ") || (len(z) >= i+4 && string(z[i:i+4]) == "%if ") || (len(z) >= i+8 && string(z[i:i+8]) == "%ifndef ") {
			if exclude != 0 {
				exclude++
			} else {
				for j = i; j < len(z) && !unicode.IsSpace(z[j]); j++ {
				}
				iBool := j
				isNot := (j == i+7)
				for j < len(z) && z[j] != '\n' {
					j++
				}
				exclude = eval_preprocessor_boolean(z[iBool:j], lineno)
				if !isNot {
					if exclude == 0 {
						exclude = 1
					} else {
						exclude = 0
					}
				}
				if exclude != 0 {
					start = i
					start_lineno = lineno
				}
			}
			for j := i; j <= len(z) && z[j] != '\n'; j++ {
				z[j] = ' '
			}
		}
	}
	if exclude != 0 {
		fmt.Fprintf(os.Stderr, "unterminated %%ifdef starting on line %d\n", start_lineno)
		os.Exit(1)
	}
}

/* In spite of its name, this function is really a scanner.  It read
** in the entire input file (all at once) then tokenizes it.  Each
** token is passed to the function "parseonetoken" which builds all
** the appropriate data structures in the global state vector "gp".
 */
func Parse(gp *lemon) {
	var ps pstate
	var startline int

	ps.gp = gp
	ps.filename = gp.filename
	ps.errorcnt = 0
	ps.state = INITIALIZE

	/* Begin by reading the input file */
	bytes, err := os.ReadFile(ps.filename)
	if err != nil {
		ErrorMsg(ps.filename, 0, fmt.Sprintf("Can't read file: %v", err))
		gp.errorcnt++
		return
	}

	filebuf := []rune(string(bytes))

	/* Make an initial pass through the file to handle %ifdef and %ifndef */
	preprocess_input(filebuf)
	if gp.printPreprocessed {
		fmt.Printf("%s\n", string(filebuf))
		return
	}

	/* Now scan the text of the input file */
	lineno := 1
	nextcp := 0
	for cp := 0; cp < len(filebuf); {
		c := filebuf[cp]

		/* Keep track of the line number */
		if c == '\n' {
			lineno++
		}

		/* Skip all white space */
		if unicode.IsSpace(c) {
			cp++
			continue
		}

		var cp1 rune
		if cp < len(filebuf)-1 {
			cp1 = filebuf[cp+1]
		}

		/* Skip C++ style comments */
		if c == '/' && cp1 == '/' {
			cp += 2
			for ; cp < len(filebuf) && filebuf[cp] != '\n'; cp++ {
			}
			continue
		}

		if c == '/' && cp1 == '*' { /* Skip C style comments */
			cp += 2
			for ; cp < len(filebuf) && (filebuf[cp] != '/' || filebuf[cp-1] != '*'); cp++ {
				if filebuf[cp] == '\n' {
					lineno++
				}
			}
			if cp < len(filebuf) {
				cp++
			}
			continue
		}

		ps.tokenstart = cp      /* Mark the beginning of the token */
		ps.tokenlineno = lineno /* Linenumber on which token begins */

		var cp2 rune
		if cp < len(filebuf)-2 {
			cp2 = filebuf[cp+2]
		}

		if c == '"' { /* String literals */
			cp++
			for ; cp < len(filebuf) && filebuf[cp] != '"'; cp++ {
				if filebuf[cp] == '\n' {
					lineno++
				}
			}
			if cp == len(filebuf) {
				ErrorMsg(ps.filename, startline, "String starting on this line is not terminated before the end of the file.")
				ps.errorcnt++
				nextcp = cp
			} else {
				nextcp = cp + 1
			}
		} else if c == '{' { /* A block of C code */
			cp++
			for level := 1; cp < len(filebuf) && (level > 1 || filebuf[cp] != '}'); cp++ {
				c = filebuf[cp]
				cp1 = 0
				if cp < len(filebuf)-1 {
					cp1 = filebuf[cp+1]
				}

				if c == '\n' {
					lineno++
				} else if c == '{' {
					level++
				} else if c == '}' {
					level--
				} else if c == '/' && cp1 == '*' {
					/* Skip comments */
					cp = cp + 2
					prevc := rune(0)
					for ; cp < len(filebuf) && (filebuf[cp] != '/' || prevc != '*'); cp++ {
						if filebuf[cp] == '\n' {
							lineno++
						}
						prevc = filebuf[cp]
					}
				} else if c == '/' && cp1 == '/' {
					/* Skip C++ style comments too */
					cp = cp + 2
					for ; cp <= len(filebuf) && filebuf[cp] != '\n'; cp++ {
					}
					if cp <= len(filebuf) {
						lineno++
					}
				} else if c == '\'' || c == '"' || c == '`' {
					/* String a character literals */
					startchar := c
					prevc := rune(0)
					for cp++; cp < len(filebuf) && (filebuf[cp] != startchar || prevc == '\\'); cp++ {
						if filebuf[cp] == '\n' {
							lineno++
						}
						if prevc == '\\' {
							prevc = 0
						} else {
							prevc = filebuf[cp]
						}
					}
				}
			}
			if cp >= len(filebuf) {
				ErrorMsg(ps.filename, ps.tokenlineno, "C code starting on this line is not terminated before the end of the file.")
				ps.errorcnt++
				nextcp = cp
			} else {
				nextcp = cp + 1
			}
		} else if isalnum(c) { /* Identifiers */
			for ; cp < len(filebuf) && (isalnum(filebuf[cp]) || filebuf[cp] == '_'); cp++ {
			}
			nextcp = cp
		} else if c == ':' && cp1 == ':' && cp2 == '=' { /* The operator "::=" */
			cp += 3
			nextcp = cp
		} else if (c == '/' || c == '|') && unicode.IsLetter(cp1) {
			cp += 2
			for ; cp < len(filebuf) && (isalnum(filebuf[cp]) || filebuf[cp] == '_'); cp++ {
			}
			nextcp = cp
		} else { /* All other (one character) operators */
			cp++
			nextcp = cp
		}
		parseonetoken(&ps, filebuf[ps.tokenstart:cp]) /* Parse the token */
		cp = nextcp
	}
	gp.rule = ps.firstrule
	gp.errorcnt = ps.errorcnt
}

/*************************** From the file "plink.c" *********************/
/*
** Routines processing configuration follow-set propagation links
** in the LEMON parser generator.
 */

var plink_freelist *plink

/* Allocate a new plink */
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

/* Add a plink to a plink list */
func Plink_add(plpp **plink, cfp *config) {
	newlink := Plink_new()
	newlink.next = *plpp
	*plpp = newlink
	newlink.cfp = cfp
}

/* Transfer every plink on the list "from" to the list "to" */
func Plink_copy(to **plink, from *plink) {
	var nextpl *plink
	for from != nil {
		nextpl = from.next
		from.next = *to
		*to = from
		from = nextpl
	}
}

/* Delete every plink on the list */
func Plink_delete(plp *plink) {
	var nextpl *plink

	for plp != nil {
		nextpl = plp.next
		plp.next = plink_freelist
		plink_freelist = plp
		plp = nextpl
	}
}

/*********************** From the file "report.c" **************************/
/*
** Procedures for generating reports and tables in the LEMON parser generator.
 */

/* Generate a filename with the given suffix.  Space to hold the
** name comes from malloc() and must be freed by the calling
** function.
 */
func file_makename(lemp *lemon, suffix string) string {
	filename := lemp.filename
	if outputDir != "" {
		last := strings.LastIndex(filename, "/")
		if last != -1 {
			filename = filename[last:]
		}
	}

	last := strings.LastIndex(filename, ".")
	if last != -1 {
		filename = filename[:last]
	}

	if outputDir != "" {
		return outputDir + "/" + filename + suffix
	}
	return filename + suffix
}

/* Open a file with a name based on the name of the input file,
** but with a different (specified) suffix, and return a pointer
** to the stream */
func file_open(lemp *lemon, suffix string, mode string) *os.File {
	var flag int
	switch mode {
	case "rb":
		flag = os.O_RDONLY
	case "wb":
		flag = os.O_WRONLY | os.O_TRUNC | os.O_CREATE
	default:
		assert(false, fmt.Sprintf(`want mode in {"rb,wb"}; got %q`, mode))
	}

	lemp.outname = file_makename(lemp, suffix)
	fp, err := os.OpenFile(lemp.outname, flag, 0644)
	if err != nil {
		fmt.Println(err)
		if mode == "rb" {
			return nil
		}
		fmt.Fprintf(os.Stderr, "Can't open file \"%s\".\n", lemp.outname)
		lemp.errorcnt++
		return nil
	}
	return fp
}

/* Print the text of a rule
 */
func rule_print(out io.Writer, rp *rule) {
	fmt.Fprintf(out, "%s", rp.lhs.name)
	/*
		if rp.lhsalias != "" {
			fmt.Fprintf(out, "(%s)", rp.lhsalias)
		}
	*/
	fmt.Fprintf(out, " ::=")
	for i := range rp.rhs {
		sp := rp.rhs[i]
		if sp.typ == MULTITERMINAL {
			fmt.Fprintf(out, " %s", sp.subsym[0].name)
			for j := 1; j < len(sp.subsym); j++ {
				fmt.Fprintf(out, "|%s", sp.subsym[j].name)
			}
		} else {
			fmt.Fprintf(out, " %s", sp.name)
		}
		/*
			if rp.rhsalias[i] != "" {
				fmt.Fprintf(out, "(%s)", rp.rhsalias[i])
			}
		*/
	}
}

/* Duplicate the input file without comments and without actions
** on rules */
func Reprint(lemp *lemon) {
	fmt.Printf("// Reprint of input file \"%s\".\n// Symbols:\n", lemp.filename)
	maxlen := 10
	for i := 0; i < lemp.nsymbol; i++ {
		sp := lemp.symbols[i]
		if len(sp.name) > maxlen {
			maxlen = len(sp.name)
		}
	}
	ncolumns := 76 / (maxlen + 5)
	if ncolumns < 1 {
		ncolumns = 1
	}
	skip := (lemp.nsymbol + ncolumns - 1) / ncolumns
	for i := 0; i < skip; i++ {
		fmt.Printf("//")
		for j := i; j < lemp.nsymbol; j += skip {
			sp := lemp.symbols[j]
			assert(sp.index == j, "sp.index==j")
			fmt.Printf(" %3d %-*.*s", j, maxlen, maxlen, sp.name)
		}
		fmt.Printf("\n")
	}
	for rp := lemp.rule; rp != nil; rp = rp.next {
		rule_print(os.Stdout, rp)
		fmt.Printf(".")
		if rp.precsym != nil {
			fmt.Printf(" [%s]", rp.precsym.name)
		}
		/*
			if rp.code {
				fmt.Printf("\n    %s", rp.code)
			}
		*/
		fmt.Printf("\n")
	}
}

/* Print a single rule.
 */
func RulePrint(fp *os.File, rp *rule, iCursor int) {
	fmt.Fprintf(fp, "%s ::=", rp.lhs.name)
	for i := 0; i <= len(rp.rhs); i++ {
		if i == iCursor {
			fmt.Fprintf(fp, " *")
		}
		if i == len(rp.rhs) {
			break
		}
		sp := rp.rhs[i]
		if sp.typ == MULTITERMINAL {
			fmt.Fprintf(fp, " %s", sp.subsym[0].name)
			for j := 1; j < len(sp.subsym); j++ {
				fmt.Fprintf(fp, "|%s", sp.subsym[j].name)
			}
		} else {
			fmt.Fprintf(fp, " %s", sp.name)
		}
	}
}

/* Print the rule for a configuration.
 */
func ConfigPrint(fp *os.File, cfp *config) {
	RulePrint(fp, cfp.rp, cfp.dot)
}

/* Print a set */
func SetPrint(out *os.File, set map[int]bool, lemp *lemon) {
	spacer := ""
	fmt.Fprintf(out, "%12s[", "")
	for i := 0; i < lemp.nterminal; i++ {
		if SetFind(set, i) {
			fmt.Fprintf(out, "%s%s", spacer, lemp.symbols[i].name)
			spacer = " "
		}
	}
	fmt.Fprintf(out, "]\n")
}

/* Print a plink chain */
func PlinkPrint(out *os.File, plp *plink, tag string) {
	for plp != nil {
		fmt.Fprintf(out, "%12s%s (state %2d) ", "", tag, plp.cfp.stp.statenum)
		ConfigPrint(out, plp.cfp)
		fmt.Fprintf(out, "\n")
		plp = plp.next
	}
}

/* Print an action to the given file descriptor.  Return FALSE if
** nothing was actually printed.
 */
func PrintAction(
	ap *action, /* The action to print */
	fp *os.File, /* Print the action here */
	indent int, /* Indent by this amount */
) bool {
	result := true
	switch ap.typ {
	case SHIFT:
		{
			stp := ap.x.stp
			fmt.Fprintf(fp, "%*s shift        %-7d", indent, ap.sp.name, stp.statenum)
		}

	case REDUCE:
		{
			rp := ap.x.rp
			fmt.Fprintf(fp, "%*s reduce       %-7d", indent, ap.sp.name, rp.iRule)
			RulePrint(fp, rp, -1)
		}

	case SHIFTREDUCE:
		{
			rp := ap.x.rp
			fmt.Fprintf(fp, "%*s shift-reduce %-7d", indent, ap.sp.name, rp.iRule)
			RulePrint(fp, rp, -1)
		}

	case ACCEPT:
		fmt.Fprintf(fp, "%*s accept", indent, ap.sp.name)

	case ERROR:
		fmt.Fprintf(fp, "%*s error", indent, ap.sp.name)

	case SRCONFLICT, RRCONFLICT:
		fmt.Fprintf(fp, "%*s reduce       %-7d ** Parsing conflict **",
			indent, ap.sp.name, ap.x.rp.iRule)

	case SSCONFLICT:
		fmt.Fprintf(fp, "%*s shift        %-7d ** Parsing conflict **",
			indent, ap.sp.name, ap.x.stp.statenum)

	case SH_RESOLVED:
		if showPrecedenceConflict {
			fmt.Fprintf(fp, "%*s shift        %-7d -- dropped by precedence",
				indent, ap.sp.name, ap.x.stp.statenum)
		} else {
			result = false
		}

	case RD_RESOLVED:
		if showPrecedenceConflict {
			fmt.Fprintf(fp, "%*s reduce %-7d -- dropped by precedence",
				indent, ap.sp.name, ap.x.rp.iRule)
		} else {
			result = false
		}

	case NOT_USED:
		result = false

	}
	if result && ap.spOpt != nil {
		fmt.Fprintf(fp, "  /* because %s==%s */", ap.sp.name, ap.spOpt.name)
	}
	return result
}

/* Generate the "*.out" log file */
func ReportOutput(lemp *lemon) {
	fp := file_open(lemp, ".out", "wb")
	if fp == nil {
		return
	}

	for i := 0; i < lemp.nxstate; i++ {
		stp := lemp.sorted[i]
		fmt.Fprintf(fp, "State %d:\n", stp.statenum)
		var cfp *config
		if lemp.basisflag {
			cfp = stp.bp
		} else {
			cfp = stp.cfp
		}
		for cfp != nil {
			if cfp.dot == len(cfp.rp.rhs) {
				buf := fmt.Sprintf("(%d)", cfp.rp.iRule)
				fmt.Fprintf(fp, "    %5s ", buf)
			} else {
				fmt.Fprintf(fp, "          ")
			}
			ConfigPrint(fp, cfp)
			fmt.Fprintf(fp, "\n")
			if false { // #if 0
				SetPrint(fp, cfp.fws, lemp)
				PlinkPrint(fp, cfp.fplp, "To  ")
				PlinkPrint(fp, cfp.bplp, "From")
			} // #endif
			if lemp.basisflag {
				cfp = cfp.bp
			} else {
				cfp = cfp.next
			}
		}
		fmt.Fprintf(fp, "\n")
		for ap := stp.ap; ap != nil; ap = ap.next {
			if PrintAction(ap, fp, 30) {
				fmt.Fprintf(fp, "\n")
			}
		}
		fmt.Fprintf(fp, "\n")
	}
	fmt.Fprintf(fp, "----------------------------------------------------\n")
	fmt.Fprintf(fp, "Symbols:\n")
	fmt.Fprintf(fp, "The first-set of non-terminals is shown after the name.\n\n")
	for i := 0; i < lemp.nsymbol; i++ {
		sp := lemp.symbols[i]
		fmt.Fprintf(fp, "  %3d: %s", i, sp.name)
		if sp.typ == NONTERMINAL {
			fmt.Fprintf(fp, ":")
			if sp.lambda {
				fmt.Fprintf(fp, " <lambda>")
			}
			for j := 0; j < lemp.nterminal; j++ {
				if len(sp.firstset) > 0 && SetFind(sp.firstset, j) {
					fmt.Fprintf(fp, " %s", lemp.symbols[j].name)
				}
			}
		}
		if sp.prec >= 0 {
			fmt.Fprintf(fp, " (precedence=%d)", sp.prec)
		}
		fmt.Fprintf(fp, "\n")
	}
	fmt.Fprintf(fp, "----------------------------------------------------\n")
	fmt.Fprintf(fp, "Syntax-only Symbols:\n")
	fmt.Fprintf(fp, "The following symbols never carry semantic content.\n\n")
	n := 0
	for i := 0; i < lemp.nsymbol; i++ {
		sp := lemp.symbols[i]
		if sp.bContent {
			continue
		}
		w := len(sp.name)
		if n > 0 && n+w > 75 {
			fmt.Fprintf(fp, "\n")
			n = 0
		}
		if n > 0 {
			fmt.Fprintf(fp, " ")
			n++
		}
		fmt.Fprintf(fp, "%s", sp.name)
		n += w
	}
	if n > 0 {
		fmt.Fprintf(fp, "\n")
	}
	fmt.Fprintf(fp, "----------------------------------------------------\n")
	fmt.Fprintf(fp, "Rules:\n")
	for rp := lemp.rule; rp != nil; rp = rp.next {
		fmt.Fprintf(fp, "%4d: ", rp.iRule)
		rule_print(fp, rp)
		fmt.Fprintf(fp, ".")
		if rp.precsym != nil {
			fmt.Fprintf(fp, " [%s precedence=%d]", rp.precsym.name, rp.precsym.prec)
		}
		fmt.Fprintf(fp, "\n")
	}
	fp.Close()
	return
}

/* Search for the file "name" which is in the same directory as
** the executable */
func pathsearch(argv0 string, name string, modemask int) string {
	dir := filepath.Dir(argv0)
	if dir != "." {
		return filepath.Join(dir, name)
	} else {
		path := os.Getenv("PATH")
		for _, dir := range filepath.SplitList(path) {
			if dir == "" {
				dir = "."
			}
			path := filepath.Join(dir, name)
			if exists, _ := Exists(path); exists {
				return path
			}
		}
	}
	return ""
}

/* Given an action, compute the integer value for that action
** which is to be put in the action table of the generated machine.
** Return negative if no action should be generated.
 */
func compute_action(lemp *lemon, ap *action) int {
	switch ap.typ {
	case SHIFT:
		return ap.x.stp.statenum
	case SHIFTREDUCE:
		/* Since a SHIFT is inherient after a prior REDUCE, convert any
		 ** SHIFTREDUCE action with a nonterminal on the LHS into a simple
		 ** REDUCE action: */
		if ap.sp.index >= lemp.nterminal && (lemp.errsym == nil || ap.sp.index != lemp.errsym.index) {
			return lemp.minReduce + ap.x.rp.iRule
		} else {
			return lemp.minShiftReduce + ap.x.rp.iRule
		}
	case REDUCE:
		return lemp.minReduce + ap.x.rp.iRule
	case ERROR:
		return lemp.errAction
	case ACCEPT:
		return lemp.accAction
	default:
		return -1
	}
}

/* The next cluster of routines are for reading the template file
** and writing the results to the generated parser */

/* The first function transfers data from "in" to "out" until
** a line is seen which begins with "%%".  The line number is
** tracked.
**
** if name!=0, then any word that begin with "Parse" is changed to
** begin with *name instead.
 */
func tplt_xfer(name string, in *bufio.Reader, out *os.File, lineno *int) {
	for {
		line, err := in.ReadString('\n')
		if err != nil && (err != io.EOF || line == "") {
			return
		}
		if strings.HasPrefix(line, "%%") {
			return
		}
		(*lineno)++
		iStart := 0
		runes := []rune(line)
		if name != "" {
			for i := 0; i < len(runes); i++ {
				if runesAt(runes, i, "Parse") && (i == 0 || !unicode.IsLetter(runes[i-1])) {
					if i > iStart {
						fmt.Fprintf(out, "%.*s", i-iStart, string(runes[iStart:]))
					}
					fmt.Fprintf(out, "%s", name)
					i += 4
					iStart = i + 1
				}
			}
		}
		fmt.Fprintf(out, "%s", string(runes[iStart:]))
	}
}

/* Skip forward past the header of the template file to the first "%%"
 */
func tplt_skip_header(in *bufio.Reader, lineno *int) {
	for {
		line, err := in.ReadString('\n')
		if err != nil && (err != io.EOF || line == "") {
			return
		}
		if strings.HasPrefix(line, "%%") {
			return
		}
		*lineno++
	}
}

/* The next function finds the template file and opens it, returning
** a pointer to the opened file. */
func tplt_open(lemp *lemon) *os.File {
	templatename := "lempar.go.tpl"

	/* first, see if user specified a template filename on the command line. */
	if user_templatename != "" {
		if _, err := os.ReadFile(user_templatename); err != nil {
			fmt.Fprintf(os.Stderr, "Can't find the parser driver template file (-T argument) \"%s\".\n",
				user_templatename)
			lemp.errorcnt++
			return nil
		}
		in, err := os.Open(user_templatename)
		if err != nil {
			in = nil
			fmt.Fprintf(os.Stderr, "Can't open the template file \"%s\".\n",
				user_templatename)
			lemp.errorcnt++
			return nil
		}
		return in
	}

	cpi := strings.LastIndex(lemp.filename, ".")
	var buf string
	if cpi > -1 {
		buf = fmt.Sprintf("%.*s.lt", cpi, lemp.filename)
	} else {
		buf = fmt.Sprintf("%s.lt", lemp.filename)
	}
	var tpltname string
	if _, err := os.ReadFile(buf); err == nil {
		tpltname = buf
	} else if _, err := os.ReadFile(templatename); err == nil {
		tpltname = templatename
	} else {
		tpltname = pathsearch(lemp.argv0, templatename, 0)
	}
	if tpltname == "" {
		fmt.Fprintf(os.Stderr, "Can't find the parser driver template file \"%s\".\n",
			templatename)
		lemp.errorcnt++
		return nil
	}
	in, err := os.Open(tpltname)
	if err != nil {
		in = nil
		fmt.Fprintf(os.Stderr, "Can't open the template file \"%s\".\n", tpltname)
		lemp.errorcnt++
	}
	return in
}

/* Print a #line directive line to the output file. */
func tplt_linedir(out *os.File, lineno int, filename string) {
	fmt.Fprintf(out, "//line %d \"", lineno)
	out.WriteString(strings.ReplaceAll(filename, "\\", "\\\\"))
	fmt.Fprintf(out, "\"\n")
}

/* Print a string to the file and keep the linenumber up to date */
func tplt_print(out *os.File, lemp *lemon, str string, lineno *int) {
	if str == "" {
		return
	}
	for _, r := range str {
		out.WriteString(string(r))
		if r == '\n' {
			(*lineno)++
		}
	}

	if !strings.HasSuffix(str, "\n") {
		out.WriteString("\n")
		(*lineno)++
	}
	if !lemp.nolinenosflag {
		(*lineno)++
		tplt_linedir(out, *lineno, lemp.outname)
	}
	return
}

/*
** The following routine emits code for the destructor for the
** symbol sp
 */
func emit_destructor_code(
	out *os.File,
	sp *symbol,
	lemp *lemon,
	lineno *int,
) {
	cp := ""

	if sp.typ == TERMINAL {
		cp = lemp.tokendest
		if cp == "" {
			return
		}
		fmt.Fprintf(out, "{\n")
		(*lineno)++
	} else if sp.destructor != "" {
		cp = sp.destructor
		fmt.Fprintf(out, "{\n")
		(*lineno)++
		if !lemp.nolinenosflag {
			(*lineno)++
			tplt_linedir(out, sp.destLineno, lemp.filename)
		}
	} else if lemp.vardest != "" {
		cp = lemp.vardest
		if cp == "" {
			return
		}
		fmt.Fprintf(out, "{\n")
		(*lineno)++
	} else {
		assert(false, "false // cannot happen") /* Cannot happen */
	}
	runes := []rune(cp)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '$' && i+1 < len(runes) && runes[i+1] == '$' {
			fmt.Fprintf(out, "(yypminor.yy%d)", sp.dtnum)
			i++
			continue
		}
		if runes[i] == '\n' {
			(*lineno)++
		}
		out.WriteString(string(runes[i]))
	}
	fmt.Fprintf(out, "\n")
	(*lineno)++
	if !lemp.nolinenosflag {
		(*lineno)++
		tplt_linedir(out, *lineno, lemp.outname)
	}
	fmt.Fprintf(out, "}\n")
	(*lineno)++
	return
}

/*
** Return TRUE (non-zero) if the given symbol has a destructor.
 */
func has_destructor(sp *symbol, lemp *lemon) bool {
	if sp.typ == TERMINAL {
		return lemp.tokendest != ""
	}
	return lemp.vardest != "" || sp.destructor != ""
}

/*
** Write and transform the rp->code string so that symbols are expanded.
** Populate the rp->codePrefix and rp->codeSuffix strings, as appropriate.
**
** Return 1 if the expanded code requires that "yylhsminor" local variable
** to be defined.
 */
func translate_code(lemp *lemon, rp *rule) int {
	// char *cp, *xp;
	// int i;
	var rc int           /* True if yylhsminor is used */
	var dontUseRhs0 bool /* If true, use of left-most RHS label is illegal */
	zSkip := -1          /* The rune index of the zOvwrt comment within rp.code, or -1 */
	// char lhsused = 0;      /* True if the LHS element has been used */
	// char lhsdirect;        /* True if LHS writes directly into stack */
	// char used[MAXRHS];     /* True for each RHS element which is used */
	var zLhs string   /* Convert the LHS symbol into this string */
	var zOvwrt string /* Comment that to allow LHS to overwrite RHS */

	used := make([]bool, len(rp.rhs))
	lhsused := false
	var buf bytes.Buffer

	if rp.code == "" {
		rp.code = "\n"
		rp.line = rp.ruleline
		rp.noCode = true
	} else {
		rp.noCode = false
	}

	var lhsdirect bool

	if len(rp.rhs) == 0 {
		/* If there are no RHS symbols, then writing directly to the LHS is ok */
		lhsdirect = true
	} else if len(rp.rhsalias) == 0 || rp.rhsalias[0] == "" {
		/* The left-most RHS symbol has no value.  LHS direct is ok.  But
		 ** we have to call the destructor on the RHS symbol first. */
		lhsdirect = true
		if has_destructor(rp.rhs[0], lemp) {
			buf.Reset()
			buf.WriteString(replaceNumbers("  yypParser.yy_destructor(%d,&yypParser.yystack[yypParser.yytos+ %d].minor);\n", rp.rhs[0].index, 1-len(rp.rhs)))
			rp.codePrefix = drain(&buf)
			rp.noCode = false
		}
	} else if rp.lhsalias == "" {
		/* There is no LHS value symbol. */
		lhsdirect = true
	} else if rp.lhsalias == rp.rhsalias[0] {
		/* The LHS symbol and the left-most RHS symbol are the same, so
		 ** direct writing is allowed */
		lhsdirect = true
		lhsused = true
		used[0] = true
		if rp.lhs.dtnum != rp.rhs[0].dtnum {
			ErrorMsg(lemp.filename, rp.ruleline,
				"%s(%s) and %s(%s) share the same label but have "+
					"different datatypes.",
				rp.lhs.name, rp.lhsalias, rp.rhs[0].name, rp.rhsalias[0])
			lemp.errorcnt++
		}
	} else {
		zOvwrt = fmt.Sprintf("/*%s-overwrites-%s*/", rp.lhsalias, rp.rhsalias[0])
		zSkipByte := strings.Index(rp.code, zOvwrt)
		if zSkipByte != -1 {
			zSkip = utf8.RuneCountInString(rp.code[:zSkipByte])
			/* The code contains a special comment that indicates that it is safe
			 ** for the LHS label to overwrite left-most RHS label. */
			lhsdirect = true
		} else {
			zSkip = -1
			lhsdirect = false
		}
	}
	if lhsdirect {
		zLhs = fmt.Sprintf("yypParser.yystack[yypParser.yytos+ %d].minor.yy%d", 1-len(rp.rhs), rp.lhs.dtnum)
	} else {
		rc = 1
		zLhs = fmt.Sprintf("yylhsminor.yy%d", rp.lhs.dtnum)
	}

	buf.Reset()

	runes := []rune(rp.code)
	/* This const cast is wrong but harmless, if we're careful. */
	w := 0
	for cp := 0; cp < len(runes); cp++ {
		w++
		if cp == zSkip {
			buf.WriteString(zOvwrt)
			cp += utf8.RuneCountInString(zOvwrt) - 1
			dontUseRhs0 = true
			continue
		}
		if unicode.IsLetter(runes[cp]) && (cp == 0 || (!isalnum(runes[cp-1]) && runes[cp-1] != '_')) {
			xp := cp + 1
			for ; xp < len(runes) && (isalnum(runes[xp]) || runes[xp] == '_'); xp++ {
			}
			substr := runes[cp:xp]
			if rp.lhsalias != "" && runesStringEqual(substr, rp.lhsalias) {
				buf.WriteString(zLhs)
				cp = xp
				lhsused = true
			} else {
				for i := range rp.rhs {
					if rp.rhsalias[i] != "" && runesStringEqual(substr, rp.rhsalias[i]) {
						if i == 0 && dontUseRhs0 {
							ErrorMsg(lemp.filename, rp.ruleline,
								"Label %s used after '%s'.",
								rp.rhsalias[0], zOvwrt)
							lemp.errorcnt++
						} else if cp > 0 && runes[cp-1] == '@' {
							/* If the argument is of the form @X then substituted
							 ** the token number of X, not the value of X */
							removeLastRune(&buf)
							buf.WriteString(replaceNumbers("yypParser.yystack[yypParser.yytos+ %d].major", i-len(rp.rhs)+1, 0))
						} else {
							sp := rp.rhs[i]
							var dtnum int
							if sp.typ == MULTITERMINAL {
								dtnum = sp.subsym[0].dtnum
							} else {
								dtnum = sp.dtnum
							}
							buf.WriteString(replaceNumbers("yypParser.yystack[yypParser.yytos+ %d].minor.yy%d", i-len(rp.rhs)+1, dtnum))
						}
						cp = xp
						used[i] = true
						break
					}
				}
			}
		}
		if cp < len(runes) {
			buf.WriteRune(runes[cp])
		}
	} /* End loop */

	/* Main code generation completed */
	cp := drain(&buf)
	if cp != "" {
		rp.code = cp
	}
	buf.Reset()

	/* Check to make sure the LHS has been used */
	if rp.lhsalias != "" && !lhsused {
		ErrorMsg(lemp.filename, rp.ruleline,
			"Label \"%s\" for \"%s(%s)\" is never USED.",
			rp.lhsalias, rp.lhs.name, rp.lhsalias)
		lemp.errorcnt++
	}

	/* Generate destructor code for RHS minor values which are not referenced.
	 ** Generate error messages for unused labels and duplicate labels.
	 */
	for i := range rp.rhs {
		if rp.rhsalias[i] != "" {
			if i > 0 {
				if rp.lhsalias != "" && rp.lhsalias == rp.rhsalias[i] {
					ErrorMsg(lemp.filename, rp.ruleline,
						"%s(%s) has the same label as the LHS but is not the left-most ",
						"symbol on the RHS.",
						rp.rhs[i].name, rp.rhsalias[i])
					lemp.errorcnt++
				}
				for j := 0; j < i; j++ {
					if rp.rhsalias[j] != "" && rp.rhsalias[j] == rp.rhsalias[i] {
						ErrorMsg(lemp.filename, rp.ruleline,
							"Label %s used for multiple symbols on the RHS of a rule.",
							rp.rhsalias[i])
						lemp.errorcnt++
						break
					}
				}
			}
			if !used[i] {
				ErrorMsg(lemp.filename, rp.ruleline,
					"Label %s for \"%s(%s)\" is never used.",
					rp.rhsalias[i], rp.rhs[i].name, rp.rhsalias[i])
				lemp.errorcnt++
			}
		} else if i > 0 && has_destructor(rp.rhs[i], lemp) {
			buf.WriteString(replaceNumbers("  yypParser.yy_destructor(%d,&yypParser.yystack[yypParser.yytos+ %d].minor);\n",
				rp.rhs[i].index, i-len(rp.rhs)+1))
		}
	}

	/* If unable to write LHS values directly into the stack, write the
	 ** saved LHS value now. */
	if !lhsdirect {
		buf.WriteString(replaceNumbers("  yypParser.yystack[yypParser.yytos+ %d].minor.yy%d = ", 1-len(rp.rhs), rp.lhs.dtnum))
		buf.WriteString(zLhs)
		buf.WriteString(";\n")
	}

	/* Suffix code generation complete */
	cp = drain(&buf)
	if cp != "" {
		rp.codeSuffix = cp
		rp.noCode = false
	}

	return rc
}

/*
** Generate code which executes when the rule "rp" is reduced.  Write
** the code to "out".  Make sure lineno stays up-to-date.
 */
func emit_code(
	out *os.File,
	rp *rule,
	lemp *lemon,
	lineno *int,
) {
	addNewlines := func(s string) {
		for _, r := range s {
			if r == '\n' {
				*lineno++
			}
		}
	}

	/* Setup code prior to the #line directive */
	if rp.codePrefix != "" {
		fmt.Fprintf(out, "{%s", rp.codePrefix)
		addNewlines(rp.codePrefix)
	}

	/* Generate code to do the reduce action */
	if rp.code != "" {
		if !lemp.nolinenosflag {
			(*lineno)++
			tplt_linedir(out, rp.line, lemp.filename)
		}
		fmt.Fprintf(out, "{%s", rp.code)
		addNewlines(rp.code)
		fmt.Fprintf(out, "}\n")
		(*lineno)++
		if !lemp.nolinenosflag {
			(*lineno)++
			tplt_linedir(out, *lineno, lemp.outname)
		}
	}

	/* Generate breakdown code that occurs after the #line directive */
	if rp.codeSuffix != "" {
		fmt.Fprintf(out, "%s", rp.codeSuffix)
		addNewlines(rp.codeSuffix)
	}

	if rp.codePrefix != "" {
		fmt.Fprintf(out, "}\n")
		(*lineno)++
	}
}

/*
** Print the definition of the union used for the parser's data stack.
** This union contains fields for every possible data type for tokens
** and nonterminals.  In the process of computing and printing this
** union, also set the ".dtnum" field of every terminal and nonterminal
** symbol.
 */
func print_stack_union(
	out *os.File, /* The output stream */
	lemp *lemon, /* The main info structure for this parser */
	plineno *int, /* Pointer to the line number */
) {
	/* Allocate and initialize types[] and allocate stddt[] */
	arraysize := lemp.nsymbol * 2
	types := make([]string, arraysize)

	var stddt string

	/* Build a hash table of datatypes. The ".dtnum" field of each symbol
	 ** is filled in with the hash index plus 1.  A ".dtnum" value of 0 is
	 ** used for terminal symbols.  If there is no %default_type defined then
	 ** 0 is also used as the .dtnum value for nonterminals which do not specify
	 ** a datatype using the %type directive.
	 */
	for i := 0; i < lemp.nsymbol; i++ {
		sp := lemp.symbols[i]
		if sp == lemp.errsym {
			sp.dtnum = arraysize + 1
			continue
		}
		if sp.typ != NONTERMINAL || (sp.datatype == "" && lemp.vartype == "") {
			sp.dtnum = 0
			continue
		}
		cp := sp.datatype
		if cp == "" {
			cp = lemp.vartype
		}
		stddt = strings.TrimSpace(cp)
		if lemp.tokentype != "" && stddt == lemp.tokentype {
			sp.dtnum = 0
			continue
		}
		hash := 0
		for _, r := range stddt {
			hash = hash*53 + int(r)
		}
		hash = (hash & 0x7fffffff) % arraysize
		for types[hash] != "" {
			if types[hash] == stddt {
				sp.dtnum = hash + 1
				break
			}
			hash++
			if hash >= arraysize {
				hash = 0
			}
		}
		if types[hash] == "" {
			sp.dtnum = hash + 1
			types[hash] = stddt
		}
	}

	/* Print out the definition of YYTOKENTYPE and YYMINORTYPE */
	name := lemp.name
	if name == "" {
		name = "Parse"
	}
	lineno := *plineno
	tokentype := lemp.tokentype
	if tokentype == "" {
		tokentype = "void*"
	}
	fmt.Fprintf(out, "type %sTOKENTYPE = %s\n", name, tokentype)
	lineno++
	fmt.Fprintf(out, "type YYMINORTYPE struct {\n")
	lineno++
	fmt.Fprintf(out, "\tyyinit int\n")
	lineno++
	fmt.Fprintf(out, "\tyy0    %sTOKENTYPE\n", name)
	lineno++
	for i := 0; i < arraysize; i++ {
		if types[i] == "" {
			continue
		}
		fmt.Fprintf(out, "\tyy%d %s\n", i+1, types[i])
		lineno++
	}
	if lemp.errsym != nil && lemp.errsym.useCnt != 0 {
		fmt.Fprintf(out, "\tyy%d   int\n", lemp.errsym.dtnum)
		lineno++
	}
	fmt.Fprintf(out, "}\n\n")
	lineno += 2
	*plineno = lineno
}

/*
** Return the name of a C datatype able to represent values between
** lwr and upr, inclusive.  If pnByte!=NULL then also write the sizeof
** for that type (1, 2, or 4) into *pnByte.
 */
func minimum_size_type(lwr int, upr int, pnByte *int) string {
	zType := "int32"
	nByte := 4
	if lwr >= 0 {
		if upr <= 255 {
			zType = "uint8"
			nByte = 1
		} else if upr < 65535 {
			zType = "uint16"
			nByte = 2
		} else {
			zType = "uint32"
			nByte = 4
		}
	} else if lwr >= -127 && upr <= 127 {
		zType = "int8"
		nByte = 1
	} else if lwr >= -32767 && upr < 32767 {
		zType = "int16"
		nByte = 2
	}
	if pnByte != nil {
		*pnByte = nByte
	}
	return zType
}

/*
** Each state contains a set of token transaction and a set of
** nonterminal transactions.  Each of these sets makes an instance
** of the following structure.  An array of these structures is used
** to order the creation of entries in the yy_action[] table.
 */
type axset struct {
	stp     *state /* A pointer to a state */
	isTkn   bool   /* True to use tokens.  False for non-terminals */
	nAction int    /* Number of actions */
	iOrder  int    /* Original order of action sets */
}

/*
** Compare to axset structures for sorting purposes
 */
func axset_compare(p1, p2 *axset) int {
	c := p2.nAction - p1.nAction
	if c == 0 {
		c = p1.iOrder - p2.iOrder
	}
	assert(c != 0 || p1 == p2, "c != 0 || p1 == p2")
	return c
}

/*
** Write text on "out" that describes the rule "rp".
 */
func writeRuleText(out *os.File, rp *rule) {
	fmt.Fprintf(out, "%s ::=", rp.lhs.name)
	for _, sp := range rp.rhs {
		if sp.typ != MULTITERMINAL {
			fmt.Fprintf(out, " %s", sp.name)
		} else {
			fmt.Fprintf(out, " %s", sp.subsym[0].name)
			for _, ss := range sp.subsym[1:] {
				fmt.Fprintf(out, "|%s", ss.name)
			}
		}
	}
}

/* Generate C source code for the parser */
func ReportTable(lemp *lemon,
	sqlFlag bool, /* Generate the *.sql file too */
) {
	var sql *os.File
	var szActionType int /* sizeof(YYACTIONTYPE) */
	var szCodeType int   /* sizeof(YYCODETYPE)   */
	var sz int
	defines := &defines{}

	lemp.minShiftReduce = lemp.nstate
	lemp.errAction = lemp.minShiftReduce + lemp.nrule
	lemp.accAction = lemp.errAction + 1
	lemp.noAction = lemp.accAction + 1
	lemp.minReduce = lemp.noAction + 1
	lemp.maxAction = lemp.minReduce + lemp.nrule

	inFile := tplt_open(lemp)
	if inFile == nil {
		return
	}
	out := file_open(lemp, ".go", "wb")
	if out == nil {
		inFile.Close()
		return
	}

	if !sqlFlag {
		sql = nil
	} else {
		sql = file_open(lemp, ".sql", "wb")
		if sql == nil {
			inFile.Close()
			out.Close()
			return
		}
		fmt.Fprintf(sql,
			"BEGIN;\n"+
				"CREATE TABLE symbol(\n"+
				"  id INTEGER PRIMARY KEY,\n"+
				"  name TEXT NOT NULL,\n"+
				"  isTerminal BOOLEAN NOT NULL,\n"+
				"  fallback INTEGER REFERENCES symbol"+
				" DEFERRABLE INITIALLY DEFERRED\n"+
				");\n",
		)
		for i := 0; i < lemp.nsymbol; i++ {
			fallback := "FALSE"
			if i < lemp.nterminal {
				fallback = "TRUE"
			}

			fmt.Fprintf(sql,
				"INSERT INTO symbol(id,name,isTerminal,fallback)"+
					"VALUES(%d,'%s',%s",
				i, lemp.symbols[i].name,
				fallback,
			)
			if lemp.symbols[i].fallback != nil {
				fmt.Fprintf(sql, ",%d);\n", lemp.symbols[i].fallback.index)
			} else {
				fmt.Fprintf(sql, ",NULL);\n")
			}
		}
		fmt.Fprintf(sql,
			"CREATE TABLE rule(\n"+
				"  ruleid INTEGER PRIMARY KEY,\n"+
				"  lhs INTEGER REFERENCES symbol(id),\n"+
				"  txt TEXT\n"+
				");\n"+
				"CREATE TABLE rulerhs(\n"+
				"  ruleid INTEGER REFERENCES rule(ruleid),\n"+
				"  pos INTEGER,\n"+
				"  sym INTEGER REFERENCES symbol(id)\n"+
				");\n",
		)
		for i, rp := 0, lemp.rule; rp != nil; rp, i = rp.next, i+1 {
			assert(i == rp.iRule, "i==rp.iRule")
			fmt.Fprintf(sql,
				"INSERT INTO rule(ruleid,lhs,txt)VALUES(%d,%d,'",
				rp.iRule, rp.lhs.index,
			)
			writeRuleText(sql, rp)
			fmt.Fprintf(sql, "');\n")
			for j := range rp.rhs {
				sp := rp.rhs[j]
				if sp.typ != MULTITERMINAL {
					fmt.Fprintf(sql,
						"INSERT INTO rulerhs(ruleid,pos,sym)VALUES(%d,%d,%d);\n",
						i, j, sp.index,
					)
				} else {
					for k := range sp.subsym {
						fmt.Fprintf(sql,
							"INSERT INTO rulerhs(ruleid,pos,sym)VALUES(%d,%d,%d);\n",
							i, j, sp.subsym[k].index,
						)
					}
				}
			}
		}
		fmt.Fprintf(sql, "COMMIT;\n")
	}
	lineno := 1

	name := lemp.name
	if name == "" {
		name = "Parse"
	}

	findPrefix := func(s string) string {
		fields := strings.Fields(s)
		if len(fields) == 0 {
			return ""
		}
		return fields[0]
	}

	if lemp.arg != "" {
		prefix := findPrefix(lemp.arg)
		defines.addDefine("ParseARG_SDECL", fmt.Sprintf("%s", lemp.arg))
		defines.addDefine("ParseARG_PDECL", fmt.Sprintf(",%s", lemp.arg))
		defines.addDefine("ParseARG_PARAM", fmt.Sprintf(",%s", prefix))
		defines.addDefine("ParseARG_FETCH", fmt.Sprintf("%s := yypParser.%s; _ = %s", prefix, prefix, prefix))
		defines.addDefine("ParseARG_STORE", fmt.Sprintf("yypParser.%s=%s", prefix, prefix))
	} else {
		defines.addDefine("ParseARG_SDECL", "")
		defines.addDefine("ParseARG_PDECL", "")
		defines.addDefine("ParseARG_PARAM", "")
		defines.addDefine("ParseARG_FETCH", "")
		defines.addDefine("ParseARG_STORE", "")
	}
	if lemp.ctx != "" {
		prefix := findPrefix(lemp.ctx)
		defines.addDefine("ParseCTX_SDECL", fmt.Sprintf("%s", lemp.ctx))
		defines.addDefine("ParseCTX_PDECL", fmt.Sprintf("%s", lemp.ctx))
		defines.addDefine("ParseCTX_PARAM", fmt.Sprintf("%s", prefix))
		defines.addDefine("ParseCTX_FETCH", fmt.Sprintf("%s := yypParser.%s; _ = %s\n", prefix, prefix, prefix))
		defines.addDefine("ParseCTX_STORE", fmt.Sprintf("yypParser.%s=%s\n", prefix, prefix))
	} else {
		defines.addDefine("ParseCTX_SDECL", "")
		defines.addDefine("ParseCTX_PDECL", "")
		defines.addDefine("ParseCTX_PARAM", "")
		defines.addDefine("ParseCTX_FETCH", "")
		defines.addDefine("ParseCTX_STORE", "")
	}

	input, err := io.ReadAll(inFile)
	if err != nil {
		return
	}
	replaced := defines.replaceAll(string(input))
	in := bufio.NewReader(bytes.NewBufferString(replaced))

	fmt.Fprintf(out,
		"/* This file is automatically generated by Lemon from input grammar\n"+
			"** source file \"%s\". */\n", lemp.filename)
	lineno += 2

	/* The first %include directive begins with a C-language comment,
	 ** then skip over the header comment of the template file
	 */
	includeRunes := []rune(lemp.include)
	for i := 0; unicode.IsSpace(includeRunes[i]); i++ {
		if includeRunes[i] == '\n' {
			includeRunes = includeRunes[i+1:]
			lemp.include = string(includeRunes)
			i = -1
		}
	}

	if includeRunes[0] == '/' && !strings.HasPrefix(lemp.include, "//line ") {
		tplt_skip_header(in, &lineno)
	} else {
		tplt_xfer(lemp.name, in, out, &lineno)
	}
	/* Generate the include code, if any */
	tplt_print(out, lemp, lemp.include, &lineno)
	tplt_xfer(lemp.name, in, out, &lineno)
	/* Generate #defines for all tokens */
	var prefix string
	if lemp.tokenprefix != "" {
		prefix = lemp.tokenprefix
	}

	fmt.Fprintf(out, "const (\n")

	for i := 1; i < lemp.nterminal; i++ {
		fmt.Fprintf(out, "\t%s%s = %d\n", prefix, lemp.symbols[i].name, i)
		lineno++
	}
	fmt.Fprintf(out, ")\n\n")
	lineno += 2
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate the defines */
	fmt.Fprintf(out, "const YYNOCODE = %d\n\n", lemp.nsymbol)
	lineno += 2
	fmt.Fprintf(out, "type YYCODETYPE = %s\n",
		minimum_size_type(0, lemp.nsymbol, &szCodeType))
	lineno++
	fmt.Fprintf(out, "type YYACTIONTYPE = %s\n",
		minimum_size_type(0, lemp.maxAction, &szActionType))
	lineno++

	print_stack_union(out, lemp, &lineno)

	wildcard := 0
	if lemp.wildcard != nil {
		wildcard = lemp.wildcard.index
	}
	fmt.Fprintf(out, "const YYWILDCARD = %d\n", wildcard)
	lineno++
	if lemp.stacksize != "" {
		fmt.Fprintf(out, "const YYSTACKDEPTH = %s\n", lemp.stacksize)
		lineno++
	} else {
		fmt.Fprintf(out, "const YYSTACKDEPTH = 100\n")
		lineno++
	}
	fmt.Fprintf(out, "const YYNOERRORRECOVERY = false\n")
	lineno++
	fmt.Fprintf(out, "const YYCOVERAGE = false\n")
	lineno++
	fmt.Fprintf(out, "const YYTRACKMAXSTACKDEPTH = false\n")
	lineno++
	fmt.Fprintf(out, "const NDEBUG = false\n")
	lineno++

	errsym := 0
	if lemp.errsym != nil && lemp.errsym.useCnt != 0 {
		errsym = lemp.errsym.index
	}
	fmt.Fprintf(out, "const YYERRORSYMBOL = %d\n", errsym)
	lineno++

	fmt.Fprintf(out, "const YYFALLBACK = %v\n", lemp.has_fallback)
	lineno++

	/* Compute the action table, but do not output it yet.  The action
	 ** table must be computed before generating the YYNSTATE macro because
	 ** we need to know how many states can be eliminated.
	 */
	ax := make([]axset, lemp.nxstate*2)
	for i := 0; i < lemp.nxstate; i++ {
		stp := lemp.sorted[i]
		ax[i*2].stp = stp
		ax[i*2].isTkn = true
		ax[i*2].nAction = stp.nTknAct
		ax[i*2+1].stp = stp
		ax[i*2+1].isTkn = false
		ax[i*2+1].nAction = stp.nNtAct
	}
	var mxTknOfst, mnTknOfst int
	var mxNtOfst, mnNtOfst int
	/* In an effort to minimize the action table size, use the heuristic
	 ** of placing the largest action sets first */
	for i := 0; i < lemp.nxstate*2; i++ {
		ax[i].iOrder = i
	}
	sort.Sort(axsetSorter(ax))
	pActtab := acttab_alloc(lemp.nsymbol, lemp.nterminal)
	for i := 0; i < lemp.nxstate*2 && ax[i].nAction > 0; i++ {
		stp := ax[i].stp
		if ax[i].isTkn {
			for ap := stp.ap; ap != nil; ap = ap.next {
				if ap.sp.index >= lemp.nterminal {
					continue
				}
				action := compute_action(lemp, ap)
				if action < 0 {
					continue
				}
				acttab_action(pActtab, ap.sp.index, action)
			}
			stp.iTknOfst = acttab_insert(pActtab, true)
			if stp.iTknOfst < mnTknOfst {
				mnTknOfst = stp.iTknOfst
			}
			if stp.iTknOfst > mxTknOfst {
				mxTknOfst = stp.iTknOfst
			}
		} else {
			for ap := stp.ap; ap != nil; ap = ap.next {
				if ap.sp.index < lemp.nterminal {
					continue
				}
				if ap.sp.index == lemp.nsymbol {
					continue
				}
				action := compute_action(lemp, ap)
				if action < 0 {
					continue
				}
				acttab_action(pActtab, ap.sp.index, action)
			}
			stp.iNtOfst = acttab_insert(pActtab, false)
			if stp.iNtOfst < mnNtOfst {
				mnNtOfst = stp.iNtOfst
			}
			if stp.iNtOfst > mxNtOfst {
				mxNtOfst = stp.iNtOfst
			}
		}
		if false { // #if 0  /* Uncomment for a trace of how the yy_action[] table fills out */
			nn := 0
			for jj := 0; jj < pActtab.nAction; jj++ {
				if pActtab.aAction[jj].action < 0 {
					nn++
				}
			}
			tokenOrVar := "Var  "
			if ax[i].isTkn {
				tokenOrVar = "Token"
			}
			fmt.Printf("%4d: State %3d %s n: %2d size: %5d freespace: %d\n",
				i, stp.statenum, tokenOrVar, ax[i].nAction, pActtab.nAction, nn)
		} //#endif
	}

	/* Mark rules that are actually used for reduce actions after all
	 ** optimizations have been applied
	 */
	for rp := lemp.rule; rp != nil; rp = rp.next {
		rp.doesReduce = false
	}
	for i := 0; i < lemp.nxstate; i++ {
		for ap := lemp.sorted[i].ap; ap != nil; ap = ap.next {
			if ap.typ == REDUCE || ap.typ == SHIFTREDUCE {
				ap.x.rp.doesReduce = true
			}
		}
	}

	/* Finish rendering the constants now that the action table has
	** been computed */
	fmt.Fprintf(out, "const YYNSTATE = %d\n", lemp.nxstate)
	lineno++
	fmt.Fprintf(out, "const YYNRULE = %d\n", lemp.nrule)
	lineno++
	fmt.Fprintf(out, "const YYNRULE_WITH_ACTION = %d\n", lemp.nruleWithAction)
	lineno++
	fmt.Fprintf(out, "const YYNTOKEN = %d\n", lemp.nterminal)
	lineno++
	fmt.Fprintf(out, "const YY_MAX_SHIFT = %d\n", lemp.nxstate-1)
	lineno++
	i := lemp.minShiftReduce
	fmt.Fprintf(out, "const YY_MIN_SHIFTREDUCE = %d\n", i)
	lineno++
	i += lemp.nrule
	fmt.Fprintf(out, "const YY_MAX_SHIFTREDUCE = %d\n", i-1)
	lineno++
	fmt.Fprintf(out, "const YY_ERROR_ACTION = %d\n", lemp.errAction)
	lineno++
	fmt.Fprintf(out, "const YY_ACCEPT_ACTION = %d\n", lemp.accAction)
	lineno++
	fmt.Fprintf(out, "const YY_NO_ACTION = %d\n", lemp.noAction)
	lineno++
	fmt.Fprintf(out, "const YY_MIN_REDUCE = %d\n", lemp.minReduce)
	lineno++
	i = lemp.minReduce + lemp.nrule
	fmt.Fprintf(out, "const YY_MAX_REDUCE = %d\n", i-1)
	lineno++

	tplt_xfer(lemp.name, in, out, &lineno)

	/* Now output the action table and its associates:
	**
	**  yy_action[]        A single table containing all actions.
	**  yy_lookahead[]     A table containing the lookahead for each entry in
	**                     yy_action.  Used to detect hash collisions.
	**  yy_shift_ofst[]    For each state, the offset into yy_action for
	**                     shifting terminals.
	**  yy_reduce_ofst[]   For each state, the offset into yy_action for
	**                     shifting non-terminals after a reduce.
	**  yy_default[]       Default action for each state.
	 */

	/* Output the yy_action table */
	n := acttab_action_size(pActtab)
	lemp.nactiontab = n
	lemp.tablesize += n * szActionType
	fmt.Fprintf(out, "const YY_ACTTAB_COUNT = %d\n\n", n)
	lineno += 2
	fmt.Fprintf(out, "var yy_action = []YYACTIONTYPE{\n")
	lineno++
	for i, j := 0, 0; i < n; i++ {
		action := acttab_yyaction(pActtab, i)
		if action < 0 {
			action = lemp.noAction
		}
		if j == 0 {
			fmt.Fprintf(out, "\t/* %d */", i)
		}
		fmt.Fprintf(out, " %d,", action)
		if j == 9 || i == n-1 {
			fmt.Fprintf(out, "\n")
			lineno++
			j = 0
		} else {
			j++
		}
	}
	fmt.Fprintf(out, "}\n")
	lineno++

	/* Output the yy_lookahead table */
	n = acttab_lookahead_size(pActtab)
	lemp.nlookaheadtab = n
	lemp.tablesize += n * szCodeType
	fmt.Fprintf(out, "var yy_lookahead = []YYCODETYPE{\n")
	lineno++
	i, j := 0, 0
	for i, j = 0, 0; i < n; i++ {
		la := acttab_yylookahead(pActtab, i)
		if la < 0 {
			la = lemp.nsymbol
		}
		if j == 0 {
			fmt.Fprintf(out, "\t/* %d */", i)
		}
		fmt.Fprintf(out, " %d,", la)
		if j == 9 {
			fmt.Fprintf(out, "\n")
			lineno++
			j = 0
		} else {
			j++
		}
	}
	/* Add extra entries to the end of the yy_lookahead[] table so that
	 ** yy_shift_ofst[]+iToken will always be a valid index into the array,
	 ** even for the largest possible value of yy_shift_ofst[] and iToken. */
	nLookAhead := lemp.nterminal + lemp.nactiontab
	for i < nLookAhead {
		if j == 0 {
			fmt.Fprintf(out, " /* %d */", i)
		}
		fmt.Fprintf(out, " %d,", lemp.nterminal)
		if j == 9 {
			fmt.Fprintf(out, "\n")
			lineno++
			j = 0
		} else {
			j++
		}
		i++
	}
	if j > 0 {
		fmt.Fprintf(out, "\n")
		lineno++
	}
	fmt.Fprintf(out, "}\n\n")
	lineno += 2

	/* Output the yy_shift_ofst[] table */
	n = lemp.nxstate
	for n > 0 && lemp.sorted[n-1].iTknOfst == NO_OFFSET {
		n--
	}
	fmt.Fprintf(out, "const YY_SHIFT_COUNT = %d\n", n-1)
	lineno++
	fmt.Fprintf(out, "const YY_SHIFT_MIN = %d\n", mnTknOfst)
	lineno++
	fmt.Fprintf(out, "const YY_SHIFT_MAX = %d\n", mxTknOfst)
	lineno++
	fmt.Fprintf(out, "\n")
	lineno++
	fmt.Fprintf(out, "var yy_shift_ofst = []%s{\n",
		minimum_size_type(mnTknOfst, lemp.nterminal+lemp.nactiontab, &sz))
	lineno++
	lemp.tablesize += n * sz
	for i, j = 0, 0; i < n; i++ {
		stp := lemp.sorted[i]
		ofst := stp.iTknOfst
		if ofst == NO_OFFSET {
			ofst = lemp.nactiontab
		}
		if j == 0 {
			fmt.Fprintf(out, "\t/* %d */", i)
		}
		fmt.Fprintf(out, " %d,", ofst)
		if j == 9 || i == n-1 {
			fmt.Fprintf(out, "\n")
			lineno++
			j = 0
		} else {
			j++
		}
	}
	fmt.Fprintf(out, "}\n\n")
	lineno += 2

	/* Output the yy_reduce_ofst[] table */
	n = lemp.nxstate
	for n > 0 && lemp.sorted[n-1].iNtOfst == NO_OFFSET {
		n--
	}
	fmt.Fprintf(out, "const YY_REDUCE_COUNT = %d\n", n-1)
	lineno++
	fmt.Fprintf(out, "const YY_REDUCE_MIN = %d\n", mnNtOfst)
	lineno++
	fmt.Fprintf(out, "const YY_REDUCE_MAX = %d\n", mxNtOfst)
	lineno++
	fmt.Fprintf(out, "\n")
	lineno++
	fmt.Fprintf(out, "var yy_reduce_ofst = []%s{\n",
		minimum_size_type(mnNtOfst-1, mxNtOfst, &sz))
	lineno++
	lemp.tablesize += n * sz
	for i, j = 0, 0; i < n; i++ {
		stp := lemp.sorted[i]
		ofst := stp.iNtOfst
		if ofst == NO_OFFSET {
			ofst = mnNtOfst - 1
		}
		if j == 0 {
			fmt.Fprintf(out, "\t/* %d */", i)
		}
		fmt.Fprintf(out, " %d,", ofst)
		if j == 9 || i == n-1 {
			fmt.Fprintf(out, "\n")
			lineno++
			j = 0
		} else {
			j++
		}
	}
	fmt.Fprintf(out, "}\n")
	lineno++

	/* Output the default action table */
	fmt.Fprintf(out, "var yy_default = []YYACTIONTYPE{\n")
	lineno++
	n = lemp.nxstate
	lemp.tablesize += n * szActionType
	for i, j = 0, 0; i < n; i++ {
		stp := lemp.sorted[i]
		if j == 0 {
			fmt.Fprintf(out, "\t/* %d */", i)
		}
		if stp.iDfltReduce < 0 {
			fmt.Fprintf(out, " %d,", lemp.errAction)
		} else {
			fmt.Fprintf(out, " %d,", stp.iDfltReduce+lemp.minReduce)
		}
		if j == 9 || i == n-1 {
			fmt.Fprintf(out, "\n")
			lineno++
			j = 0
		} else {
			j++
		}
	}
	fmt.Fprintf(out, "}\n")
	lineno++
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate the table of fallback tokens.
	 */
	if lemp.has_fallback {
		mx := lemp.nterminal - 1
		/* 2019-08-28:  Generate fallback entries for every token to avoid
		 ** having to do a range check on the index */
		/* for mx>0 && lemp.symbols[mx].fallback==nil { mx--; } */
		lemp.tablesize += (mx + 1) * szCodeType
		for i = 0; i <= mx; i++ {
			p := lemp.symbols[i]
			if p.fallback == nil {
				fmt.Fprintf(out, "\t0,  /* %10s => nothing */\n", p.name)
			} else {
				fmt.Fprintf(out, "\t%d,  /* %10s => %s */\n", p.fallback.index,
					p.name, p.fallback.name)
			}
			lineno++
		}
	}
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate a table containing the symbolic name of every symbol
	 */
	for i = 0; i < lemp.nsymbol; i++ {
		// lemon_sprintf(line,"\"%s\",",lemp.symbols[i].name);
		fmt.Fprintf(out, "\t/* %4d */ \"%s\",\n", i, lemp.symbols[i].name)
		lineno++
	}
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate a table containing a text string that describes every
	 ** rule in the rule set of the grammar.  This information is used
	 ** when tracing REDUCE actions.
	 */
	for i, rp := 0, lemp.rule; rp != nil; rp, i = rp.next, i+1 {
		assert(rp.iRule == i, "rp.iRule == i")
		fmt.Fprintf(out, "\t/* %3d */ \"", i)
		writeRuleText(out, rp)
		fmt.Fprintf(out, "\",\n")
		lineno++
	}
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate code which executes every time a symbol is popped from
	 ** the stack while processing errors or while destroying the parser.
	 ** (In other words, generate the %destructor actions)
	 */
	if lemp.tokendest != "" {
		once := true
		for i := 0; i < lemp.nsymbol; i++ {
			sp := lemp.symbols[i]
			if sp == nil || sp.typ != TERMINAL {
				continue
			}
			if once {
				fmt.Fprintf(out, "      /* TERMINAL Destructor */\n")
				lineno++
				once = false
			}
			fmt.Fprintf(out, "    case %d: /* %s */\n", sp.index, sp.name)
			lineno++
		}
		for i = 0; i < lemp.nsymbol && lemp.symbols[i].typ != TERMINAL; i++ {
		}
		if i < lemp.nsymbol {
			emit_destructor_code(out, lemp.symbols[i], lemp, &lineno)
			fmt.Fprintf(out, "      break\n")
			lineno++
		}
	}
	if lemp.vardest != "" {
		var dflt_sp *symbol
		once := true
		for i := 0; i < lemp.nsymbol; i++ {
			sp := lemp.symbols[i]
			if sp == nil || sp.typ == TERMINAL ||
				sp.index <= 0 || sp.destructor != "" {
				continue
			}
			if once {
				fmt.Fprintf(out, "      /* Default NON-TERMINAL Destructor */\n")
				lineno++
				once = false
			}
			fmt.Fprintf(out, "    case %d: /* %s */\n", sp.index, sp.name)
			lineno++
			dflt_sp = sp
		}
		if dflt_sp != nil {
			emit_destructor_code(out, dflt_sp, lemp, &lineno)
		}
		fmt.Fprintf(out, "      break\n")
		lineno++
	}
	for i := 0; i < lemp.nsymbol; i++ {
		sp := lemp.symbols[i]
		if sp == nil || sp.typ == TERMINAL || sp.destructor == "" {
			continue
		}
		if sp.destLineno < 0 {
			continue /* Already emitted */
		}
		fmt.Fprintf(out, "    case %d: /* %s */\n", sp.index, sp.name)
		lineno++

		/* Combine duplicate destructors into a single case */
		for j := i + 1; j < lemp.nsymbol; j++ {
			sp2 := lemp.symbols[j]
			if sp2 != nil && sp2.typ != TERMINAL && sp2.destructor != "" &&
				sp2.dtnum == sp.dtnum &&
				sp.destructor == sp2.destructor {
				fmt.Fprintf(out, "    case %d: /* %s */\n",
					sp2.index, sp2.name)
				lineno++
				sp2.destLineno = -1 /* Avoid emitting this destructor again */
			}
		}

		emit_destructor_code(out, lemp.symbols[i], lemp, &lineno)
		fmt.Fprintf(out, "      break\n")
		lineno++
	}
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate code which executes whenever the parser stack overflows */
	tplt_print(out, lemp, lemp.overflow, &lineno)
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate the tables of rule information.  yyRuleInfoLhs[] and
	 ** yyRuleInfoNRhs[].
	 **
	 ** Note: This code depends on the fact that rules are number
	 ** sequentially beginning with 0.
	 */
	for i, rp := 0, lemp.rule; rp != nil; rp, i = rp.next, i+1 {
		fmt.Fprintf(out, "\t%d, /* (%d) ", rp.lhs.index, i)
		rule_print(out, rp)
		fmt.Fprintf(out, " */\n")
		lineno++
	}
	tplt_xfer(lemp.name, in, out, &lineno)
	for i, rp := 0, lemp.rule; rp != nil; rp, i = rp.next, i+1 {
		fmt.Fprintf(out, "\t%d, /* (%d) ", -len(rp.rhs), i)
		rule_print(out, rp)
		fmt.Fprintf(out, " */\n")
		lineno++
	}
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate code which execution during each REDUCE action */
	i = 0
	for rp := lemp.rule; rp != nil; rp = rp.next {
		i += translate_code(lemp, rp)
	}
	/* First output rules other than the default: rule */
	for rp := lemp.rule; rp != nil; rp = rp.next {
		if rp.codeEmitted {
			continue
		}
		if rp.noCode {
			/* No C code actions, so this will be part of the "default:" rule */
			continue
		}
		fmt.Fprintf(out, "      case %d: /* ", rp.iRule)
		writeRuleText(out, rp)
		fmt.Fprintf(out, " */\n")
		lineno++
		for rp2 := rp.next; rp2 != nil; rp2 = rp2.next { /* Other rules with the same action */
			if rp2.code == rp.code && rp2.codePrefix == rp.codePrefix && rp2.codeSuffix == rp.codeSuffix {
				fmt.Fprintf(out, "        fallthrough\n")
				lineno++
				fmt.Fprintf(out, "      case %d: /* ", rp2.iRule)
				writeRuleText(out, rp2)
				fmt.Fprintf(out, " */ yytestcase(yyruleno==%d);\n", rp2.iRule)
				lineno++
				rp2.codeEmitted = true
			}
		}
		emit_code(out, rp, lemp, &lineno)
		fmt.Fprintf(out, "        break\n")
		lineno++
		rp.codeEmitted = true
	}
	/* Finally, output the default: rule.  We choose as the default: all
	 ** empty actions. */
	fmt.Fprintf(out, "\tdefault:\n")
	lineno++
	for rp := lemp.rule; rp != nil; rp = rp.next {
		if rp.codeEmitted {
			continue
		}
		assert(rp.noCode, "rp.noCode")
		fmt.Fprintf(out, "\t\t/* (%d) ", rp.iRule)
		writeRuleText(out, rp)
		if rp.neverReduce {
			fmt.Fprintf(out, " (NEVER REDUCES) */ assert(yyruleno!=%d)\n",
				rp.iRule)
			lineno++
		} else if rp.doesReduce {
			fmt.Fprintf(out, " */ yytestcase(yyruleno == %d)\n", rp.iRule)
			lineno++
		} else {
			fmt.Fprintf(out, " (OPTIMIZED OUT) */ assert(yyruleno!=%d, \"yyruleno!=%d\")\n",
				rp.iRule, rp.iRule)
			lineno++
		}
	}
	fmt.Fprintf(out, "\t\tbreak\n")
	lineno++
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate code which executes if a parse fails */
	tplt_print(out, lemp, lemp.failure, &lineno)
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate code which executes when a syntax error occurs */
	tplt_print(out, lemp, lemp.error, &lineno)
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Generate code which executes when the parser accepts its input */
	tplt_print(out, lemp, lemp.accept, &lineno)
	tplt_xfer(lemp.name, in, out, &lineno)

	/* Append any addition code the user desires */
	tplt_print(out, lemp, lemp.extracode, &lineno)

	// acttab_free(pActtab)
	inFile.Close()
	out.Close()
	if sql != nil {
		sql.Close()
	}
}

/* Reduce the size of the action tables, if possible, by making use
** of defaults.
**
** In this version, we take the most frequent REDUCE action and make
** it the default.  Except, there is no default if the wildcard token
** is a possible look-ahead.
 */
func CompressTables(lemp *lemon) {
	var nbest int
	var rbest *rule
	var usesWildcard bool
	for i := 0; i < lemp.nstate; i++ {
		stp := lemp.sorted[i]
		nbest = 0
		rbest = nil
		usesWildcard = false

		for ap := stp.ap; ap != nil; ap = ap.next {
			if ap.typ == SHIFT && ap.sp == lemp.wildcard {
				usesWildcard = true
			}
			if ap.typ != REDUCE {
				continue
			}
			rp := ap.x.rp
			if rp.lhsStart {
				continue
			}
			if rp == rbest {
				continue
			}
			n := 1
			for ap2 := ap.next; ap2 != nil; ap2 = ap2.next {
				if ap2.typ != REDUCE {
					continue
				}
				rp2 := ap2.x.rp
				if rp2 == rbest {
					continue
				}
				if rp2 == rp {
					n++
				}
			}
			if n > nbest {
				nbest = n
				rbest = rp
			}
		}

		/* Do not make a default if the number of rules to default
		 ** is not at least 1 or if the wildcard token is a possible
		 ** lookahead.
		 */
		if nbest < 1 || usesWildcard {
			continue
		}

		/* Combine matching REDUCE actions into a single default */
		var ap *action
		for ap = stp.ap; ap != nil; ap = ap.next {
			if ap.typ == REDUCE && ap.x.rp == rbest {
				break
			}
		}
		assert(ap != nil, "ap!=nil")
		ap.sp = Symbol_new("{default}")
		for ap = ap.next; ap != nil; ap = ap.next {
			if ap.typ == REDUCE && ap.x.rp == rbest {
				ap.typ = NOT_USED
			}
		}
		stp.ap = Action_sort(stp.ap)

		for ap = stp.ap; ap != nil; ap = ap.next {
			if ap.typ == SHIFT {
				break
			}
			if ap.typ == REDUCE && ap.x.rp != rbest {
				break
			}
		}
		if ap == nil {
			stp.autoReduce = true
			stp.pDfltReduce = rbest
		}
	}

	/* Make a second pass over all states and actions.  Convert
	 ** every action that is a SHIFT to an autoReduce state into
	 ** a SHIFTREDUCE action.
	 */
	for i := 0; i < lemp.nstate; i++ {
		stp := lemp.sorted[i]
		for ap := stp.ap; ap != nil; ap = ap.next {
			if ap.typ != SHIFT {
				continue
			}
			pNextState := ap.x.stp
			if pNextState.autoReduce && pNextState.pDfltReduce != nil {
				ap.typ = SHIFTREDUCE
				ap.x.rp = pNextState.pDfltReduce
			}
		}
	}

	/* If a SHIFTREDUCE action specifies a rule that has a single RHS term
	 ** (meaning that the SHIFTREDUCE will land back in the state where it
	 ** started) and if there is no C-code associated with the reduce action,
	 ** then we can go ahead and convert the action to be the same as the
	 ** action for the RHS of the rule.
	 */
	for i := 0; i < lemp.nstate; i++ {
		stp := lemp.sorted[i]
		var nextap *action
		for ap := stp.ap; ap != nil; ap = nextap {
			nextap = ap.next
			if ap.typ != SHIFTREDUCE {
				continue
			}
			rp := ap.x.rp
			if !rp.noCode {
				continue
			}
			if len(rp.rhs) != 1 {
				continue
			}
			// #if 1
			/* Only apply this optimization to non-terminals.  It would be OK to
			 ** apply it to terminal symbols too, but that makes the parser tables
			 ** larger. */
			if ap.sp.index < lemp.nterminal {
				continue
			}
			// #endif
			/* If we reach this point, it means the optimization can be applied */
			nextap = ap
			var ap2 *action
			for ap2 = stp.ap; ap2 != nil && (ap2 == ap || ap2.sp != rp.lhs); ap2 = ap2.next {
			}
			assert(ap2 != nil, "ap2!=nil")
			ap.spOpt = ap2.sp
			ap.typ = ap2.typ
			ap.x = ap2.x
		}
	}
}

/*
** Compare two states for sorting purposes.  The smaller state is the
** one with the most non-terminal actions.  If they have the same number
** of non-terminal actions, then the smaller is the one with the most
** token actions.
 */
func stateResortCompare(pA *state, pB *state) int {

	n := pB.nNtAct - pA.nNtAct
	if n == 0 {
		n = pB.nTknAct - pA.nTknAct
		if n == 0 {
			n = pB.statenum - pA.statenum
		}
	}
	assert(n != 0, "n!=0")
	return n
}

/*
** Renumber and resort states so that states with fewer choices
** occur at the end.  Except, keep state 0 as the first state.
 */

type stateResortSorter []*state

var _ sort.Interface = stateResortSorter(nil)

func (s stateResortSorter) Len() int           { return len(s) }
func (s stateResortSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s stateResortSorter) Less(i, j int) bool { return stateResortCompare(s[i], s[j]) < 0 }

func ResortStates(lemp *lemon) {
	var stp *state

	for i := 0; i < lemp.nstate; i++ {
		stp = lemp.sorted[i]
		stp.nTknAct = 0
		stp.nNtAct = 0
		stp.iDfltReduce = -1 /* Init dflt action to "syntax error" */
		stp.iTknOfst = NO_OFFSET
		stp.iNtOfst = NO_OFFSET
		for ap := stp.ap; ap != nil; ap = ap.next {
			iAction := compute_action(lemp, ap)
			if iAction >= 0 {
				if ap.sp.index < lemp.nterminal {
					stp.nTknAct++
				} else if ap.sp.index < lemp.nsymbol {
					stp.nNtAct++
				} else {
					assert(!stp.autoReduce || stp.pDfltReduce == ap.x.rp, "!stp.autoReduce || stp.pDfltReduce==ap.x.rp")
					stp.iDfltReduce = iAction
				}
			}
		}
	}

	// qsort(&lemp.sorted[1], lemp.nstate-1, sizeof(lemp.sorted[0]),
	//       stateResortCompare);
	sort.Sort(stateResortSorter(lemp.sorted[1:]))

	for i := 0; i < lemp.nstate; i++ {
		lemp.sorted[i].statenum = i
	}
	lemp.nxstate = lemp.nstate
	for lemp.nxstate > 1 && lemp.sorted[lemp.nxstate-1].autoReduce {
		lemp.nxstate--
	}
}

/***************** From the file "set.c" ************************************/
/*
** Set manipulation routines for the LEMON parser generator.
 */

func SetNew() map[int]bool {
	return make(map[int]bool)
}

/* Add a new element to the set.  Return TRUE if the element was added
** and FALSE if it was already there. */
func SetAdd(s map[int]bool, e int) bool {
	_, found := s[e]
	s[e] = true
	return !found
}

/* Add every element of s2 to s1.  Return TRUE if s1 changes. */
func SetUnion(s1, s2 map[int]bool) (changed bool) {
	progress := false
	for k, v := range s2 {
		if !v {
			continue
		}
		if !s1[k] {
			progress = true
			s1[k] = true
		}
	}
	return progress
}

/********************** From the file "table.c" ****************************/
/*
** All code in this file has been automatically generated
** from a specification in the file
**              "table.q"
** by the associative array code building program "aagen".
** Do not edit this file!  Instead, edit the specification
** file, then rerun aagen.
 */
/*
** Code for processing tables in the LEMON parser generator.
 */

/* Return a pointer to the (terminal or nonterminal) symbol "x".
** Create a new symbol if this is the first time "x" has been seen.
 */
func Symbol_new(x string) *symbol {
	sp := Symbol_find(x)
	if sp == nil {
		typ := NONTERMINAL
		if firstRuneIsUpper(x) {
			typ = TERMINAL
		}
		sp = &symbol{
			name:       x,
			typ:        typ,
			rule:       nil,
			fallback:   nil,
			prec:       -1,
			assoc:      UNK,
			firstset:   nil,
			lambda:     false,
			destructor: "",
			destLineno: 0,
			datatype:   "",
			useCnt:     0,
		}
		Symbol_insert(sp, sp.name)
	}
	sp.useCnt++
	return sp
}

/* Compare two symbols for sorting purposes.  Return negative,
** zero, or positive if a is less then, equal to, or greater
** than b.
**
** Symbols that begin with upper case letters (terminals or tokens)
** must sort before symbols that begin with lower case letters
** (non-terminals).  And MULTITERMINAL symbols (created using the
** %token_class directive) must sort at the very end. Other than
** that, the order does not matter.
**
** We find experimentally that leaving the symbols in their original
** order (the order they appeared in the grammar file) gives the
** smallest parser tables in SQLite.
 */
func Symbolcmpp(a, b *symbol) int {
	var i1, i2 int
	switch {
	case a.typ == MULTITERMINAL:
		i1 = 3
	case a.name != "" && a.name[0] > 'Z':
		i1 = 2
	default:
		i1 = 1
	}

	switch {
	case b.typ == MULTITERMINAL:
		i2 = 3
	case b.name != "" && b.name[0] > 'Z':
		i2 = 2
	default:
		i2 = 1
	}
	if i1 == i2 {
		return a.index - b.index
	}
	return i1 - i2
}

var x2a_keys []string
var x2a = make(map[string]*symbol)

/* Allocate a new associative array */
func Symbol_init() {
	if x2a != nil {
		return
	}
	x2a = make(map[string]*symbol)
}

/* Insert a new record into the array.  Return TRUE if successful.
** Prior data with the same key is NOT overwritten */
func Symbol_insert(data *symbol, key string) bool {
	if x2a == nil {
		return false
	}
	if _, found := x2a[key]; found {
		return false
	}
	x2a_keys = append(x2a_keys, key)
	x2a[key] = data
	return true
}

/* Return a pointer to data assigned to the given key.  Return NULL
** if no such key. */
func Symbol_find(key string) *symbol {
	if x2a == nil {
		return nil
	}
	return x2a[key]
}

/* Return the size of the array */
func Symbol_count() int {
	return len(x2a)
}

/* Return an array of pointers to all data in the table.
** The array is obtained from malloc.  Return NULL if memory allocation
** problems, or if the array is empty. */
func Symbol_arrayof() []*symbol {
	result := make([]*symbol, 0, len(x2a))
	for _, key := range x2a_keys {
		result = append(result, x2a[key])
	}
	return result
}

/* Compare two configurations */
func Configcmp(a, b *config) int {
	x := a.rp.index - b.rp.index
	if x == 0 {
		x = a.dot - b.dot
	}
	return x
}

/* Compare two states */
func statecmp(a *config, b *config) int {
	var rc int
	for rc = 0; rc == 0 && a != nil && b != nil; a, b = a.bp, b.bp {
		rc = a.rp.index - b.rp.index
		if rc == 0 {
			rc = a.dot - b.dot
		}
	}
	if rc == 0 {
		if a != nil {
			rc = 1
		}
		if b != nil {
			rc = -1
		}
	}
	return rc
}

/* Hash a state */
func statehash(a *config) uint {
	var h uint
	for a != nil {
		h = h*571 + uint(a.rp.index)*37 + uint(a.dot)
		a = a.bp
	}
	return h
}

/* Allocate a new state structure */
func State_new() *state {
	return &state{}
}

/* There is one instance of the following structure for each
** associative array of type "x3".
 */
type s_x3 struct {
	size int /* The number of available slots. */
	/*   Must be a power of 2 greater than or */
	/*   equal to 1 */
	count int       /* Number of currently slots filled */
	tbl   []x3node  /* The data stored here */
	ht    []*x3node /* Hash table for lookups */
}

/* There is one instance of this structure for every data element
** in an associative array of type "x3".
 */
type x3node struct {
	data *state   /* The data */
	key  *config  /* The key */
	next *x3node  /* Next entry with the same hash */
	from **x3node /* Previous link */
}

/* There is only one instance of the array, which is the following */
var x3a *s_x3

/* Allocate a new associative array */
func State_init() {
	if x3a != nil {
		return
	}
	x3a = &s_x3{
		size:  128,
		count: 0,
		tbl:   make([]x3node, 128),
		ht:    make([]*x3node, 128),
	}
}

/* Insert a new record into the array.  Return TRUE if successful.
** Prior data with the same key is NOT overwritten */
func State_insert(data *state, key *config) bool {
	var np *x3node

	if x3a == nil {
		return false
	}
	ph := statehash(key)
	h := int(ph & uint(x3a.size-1))
	np = x3a.ht[h]
	for np != nil {
		if statecmp(np.key, key) == 0 {
			/* An existing entry with the same key is found. */
			/* Fail because overwrite is not allows. */
			return false
		}
		np = np.next
	}
	if x3a.count >= x3a.size {
		/* Need to make the hash table bigger */
		var array s_x3
		arrSize := x3a.size * 2
		array.size = arrSize
		array.count = x3a.count
		array.tbl = make([]x3node, arrSize)
		array.ht = make([]*x3node, arrSize)
		for i := 0; i < x3a.count; i++ {
			oldnp := &(x3a.tbl[i])
			h = int(statehash(oldnp.key) & uint(arrSize-1))
			newnp := &(array.tbl[i])
			if array.ht[h] != nil {
				array.ht[h].from = &(newnp.next)
			}
			newnp.next = array.ht[h]
			newnp.key = oldnp.key
			newnp.data = oldnp.data
			newnp.from = &(array.ht[h])
			array.ht[h] = newnp
		}
		*x3a = array
	}
	/* Insert the new data */
	h = int(ph & uint(x3a.size-1))
	np = &(x3a.tbl[x3a.count])
	x3a.count++
	np.key = key
	np.data = data
	if x3a.ht[h] != nil {
		x3a.ht[h].from = &(np.next)
	}
	np.next = x3a.ht[h]
	x3a.ht[h] = np
	np.from = &(x3a.ht[h])
	return true
}

/* Return a pointer to data assigned to the given key.  Return NULL
** if no such key. */
func State_find(key *config) *state {
	if x3a == nil {
		return nil
	}

	h := int(statehash(key) & uint(x3a.size-1))
	np := x3a.ht[h]
	for np != nil {
		if statecmp(np.key, key) == 0 {
			return np.data
		}
		np = np.next
	}
	return nil
}

/* Return an array of pointers to all data in the table.
** The array is obtained from malloc.  Return NULL if memory allocation
** problems, or if the array is empty. */
func State_arrayof() []*state {
	if x3a == nil {
		return nil
	}
	arrSize := x3a.count
	array := make([]*state, arrSize)
	for i := 0; i < arrSize; i++ {
		array[i] = x3a.tbl[i].data
	}
	return array
}

/* Hash a configuration */
func confighash(a *config) uint {
	var h uint
	h = h*571 + uint(a.rp.index)*37 + uint(a.dot)
	return h
}

/* There is one instance of the following structure for each
** associative array of type "x4".
 */
type s_x4 struct {
	size int /* The number of available slots. */
	/*   Must be a power of 2 greater than or */
	/*   equal to 1 */
	count int       /* Number of currently slots filled */
	tbl   []x4node  /* The data stored here */
	ht    []*x4node /* Hash table for lookups */
}

/* There is one instance of this structure for every data element
** in an associative array of type "x4".
 */
type x4node struct {
	data *config  /* The data */
	next *x4node  /* Next entry with the same hash */
	from **x4node /* Previous link */
}

/* There is only one instance of the array, which is the following */
var x4a *s_x4

/* Allocate a new associative array */
func Configtable_init() {
	if x4a != nil {
		return
	}
	x4a = &s_x4{
		size:  64,
		count: 0,
		tbl:   make([]x4node, 64),
		ht:    make([]*x4node, 64),
	}
}

/* Insert a new record into the array.  Return TRUE if successful.
** Prior data with the same key is NOT overwritten */
func Configtable_insert(data *config) bool {
	if x4a == nil {
		return false
	}
	ph := confighash(data)
	h := int(ph & uint(x4a.size-1))
	np := x4a.ht[h]
	for np != nil {
		if Configcmp(np.data, data) == 0 {
			/* An existing entry with the same key is found. */
			/* Fail because overwrite is not allows. */
			return false
		}
		np = np.next
	}
	if x4a.count >= x4a.size {
		/* Need to make the hash table bigger */
		var array s_x4
		arrSize := x4a.size * 2
		array.size = arrSize
		array.count = x4a.count
		array.tbl = make([]x4node, arrSize)
		array.ht = make([]*x4node, arrSize)
		for i := 0; i < arrSize; i++ {
			array.ht[i] = nil
		}
		for i := 0; i < x4a.count; i++ {
			oldnp := &(x4a.tbl[i])
			h := int(confighash(oldnp.data) & uint(arrSize-1))
			newnp := &(array.tbl[i])
			if array.ht[h] != nil {
				array.ht[h].from = &(newnp.next)
			}
			newnp.next = array.ht[h]
			newnp.data = oldnp.data
			newnp.from = &(array.ht[h])
			array.ht[h] = newnp
		}
		/* free(x4a.tbl); // This code was originall written for 16-bit machines.
		 ** on modern machines, don't worry about freeing this trival amount of
		 ** memory. */
		*x4a = array
	}
	/* Insert the new data */
	h = int(ph & uint(x4a.size-1))
	np = &(x4a.tbl[x4a.count])
	x4a.count++
	np.data = data
	if x4a.ht[h] != nil {
		x4a.ht[h].from = &(np.next)
	}
	np.next = x4a.ht[h]
	x4a.ht[h] = np
	np.from = &(x4a.ht[h])
	return true
}

/* Return a pointer to data assigned to the given key.  Return NULL
** if no such key. */
func Configtable_find(key *config) *config {
	if x4a == nil {
		return nil
	}
	h := int(confighash(key) & uint(x4a.size-1))
	np := x4a.ht[h]
	for np != nil {
		if Configcmp(np.data, key) == 0 {
			return np.data
		}
		np = np.next
	}
	return nil
}

/* Remove all data from the table. */
func Configtable_clear() {
	if x4a == nil || x4a.count == 0 {
		return
	}
	for i := 0; i < x4a.size; i++ {
		x4a.ht[i] = nil
	}
	x4a.count = 0
}

/// --------------------------------------------------------------------------------
/// Extras

func assert(condition bool, debug string) {
	if !condition {
		if _, file, line, ok := runtime.Caller(1); ok {
			fmt.Fprintf(os.Stderr, "%s:%d: assert failed: %s\n", file, line, debug)
		} else {
			fmt.Fprintf(os.Stderr, "assert failed: %s\n", debug)
		}
		os.Exit(1)
	}
}

func firstRuneIsUpper(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
}

/// For working with -D repeated commandline option.

type setFlag map[string]bool

func (s setFlag) String() string {
	var keys []string
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "{" + strings.Join(keys, ",") + "}"
}

func (s setFlag) Set(value string) error {
	s[value] = true
	return nil
}

func Exists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func runesAt(runes []rune, pos int, wantString string) bool {
	want := []rune(wantString)
	if pos+len(want) > len(runes) {
		return false
	}
	for i, r := range want {
		if runes[pos+i] != r {
			return false
		}
	}
	return true
}

// Helpers replacing what was append_str

func removeLastRune(buf *bytes.Buffer) {
	bb := buf.Bytes()
	l := len(bb)
	_, size := utf8.DecodeLastRune(bb)
	buf.Truncate(l - size)
}

func replaceNumbers(s string, n1 int, n2 int) string {
	parts := strings.SplitN(s, "%d", 3)
	switch len(parts) {
	case 2:
		return fmt.Sprintf("%s%d%s", parts[0], n1, parts[1])
	case 3:
		return fmt.Sprintf("%s%d%s%d%s", parts[0], n1, parts[1], n2, parts[2])
	}
	return s
}

func drain(buf *bytes.Buffer) string {
	s := buf.String()
	buf.Reset()
	return s
}

func runesStringEqual(rs []rune, s string) bool {
	count := 0

	for _, r := range s {
		if count >= len(rs) {
			return false
		}
		if r != rs[count] {
			return false
		}
		count++
	}

	if count != len(rs) {
		return false
	}

	return true
}

// Sorts

type symbolSorter []*symbol

var _ sort.Interface = symbolSorter(nil)

func (s symbolSorter) Len() int           { return len(s) }
func (s symbolSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s symbolSorter) Less(i, j int) bool { return Symbolcmpp(s[i], s[j]) < 0 }

type axsetSorter []axset

var _ sort.Interface = axsetSorter(nil)

func (s axsetSorter) Len() int           { return len(s) }
func (s axsetSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s axsetSorter) Less(i, j int) bool { return axset_compare(&s[i], &s[j]) < 0 }

func PrintSet(set map[int]bool, label string) {
	if len(set) == 0 {
		return
	}
	fmt.Printf("%s", label)
	ints := make([]int, 0, len(set))
	for k := range set {
		ints = append(ints, k)
	}
	sort.Ints(ints)
	for _, i := range ints {
		fmt.Printf(" %d", i)
	}
	fmt.Printf("\n")
}

func PrintSymbol(lemp *lemon, sp *symbol) {
	fmt.Printf("  %s lambda=%v type=%d nsubsym=%d index=%d\n", sp.name, sp.lambda, sp.typ, len(sp.subsym), sp.index)
	if len(sp.subsym) > 0 {
		fmt.Printf("    subsym: ")
		for _, ssp := range sp.subsym {
			fmt.Printf("%s ", ssp.name)
		}
		fmt.Printf("\n")
	}
	if sp.rule != nil {
		fmt.Printf("    rules:")
		for rp := sp.rule; rp != nil; rp = rp.nextlhs {
			fmt.Printf(" %s", rp.lhs.name)
		}
		fmt.Printf("\n")
	}
	PrintSet(sp.firstset, "    firstset:")
}

func PrintRule(lemp *lemon, rp *rule) {
	fmt.Printf("  %s(%d)\n", rp.lhs.name, rp.iRule)
	fmt.Printf("  rhs:")
	for _, s := range rp.rhs {
		fmt.Printf(" %s(%d)", s.name, s.index)
	}
	fmt.Printf("\n")
	if rp.nextlhs != nil {
		fmt.Printf("  nextlhs: %d\n", rp.nextlhs.iRule)
	}
}

type foostate struct {
	bp          *config /* The basis configurations for this state */
	cfp         *config /* All configurations in this set */
	statenum    int     /* Sequential number for this state */
	ap          *action /* List of actions for this state */
	nTknAct     int     /* Number of actions on terminals and nonterminals */
	nNtAct      int
	iTknOfst    int /* yyaction[] offset for terminals and nonterms */
	iNtOfst     int
	iDfltReduce int   /* Default action is to REDUCE by this rule */
	pDfltReduce *rule /* The default REDUCE rule. */
	autoReduce  bool  /* True if this is an auto-reduce state */
}

func PrintState(lemp *lemon, sp *state) {
	fmt.Printf(" %s(%d) - %d %d %d %d %v", sp.bp.rp.lhs.name, sp.statenum, sp.nTknAct, sp.nNtAct, sp.iTknOfst, sp.iDfltReduce, sp.autoReduce)
	if sp.bp != nil {
		fmt.Printf(" %d.%d", sp.bp.rp.iRule, sp.bp.dot)
	}
	if sp.pDfltReduce != nil {
		fmt.Printf(" %r", sp.pDfltReduce.iRule)
	}
	fmt.Printf("\n")
	if sp.cfp != nil {
		fmt.Printf("  cfp:")
		for cfp := sp.cfp; cfp != nil; cfp = cfp.next {
			fmt.Printf(" %d.%d", cfp.rp.iRule, cfp.dot)
		}
		fmt.Printf("\n")
	}
	if sp.ap != nil {
		fmt.Printf("  ap:")
		for ap := sp.ap; ap != nil; ap = ap.next {
			fmt.Printf(" %d", sp.ap.sp.index)
		}
		fmt.Printf("\n")
	}
}

func PrintLemon(lemp *lemon) {
	startRule := -1
	if lemp.startRule != nil {
		startRule = lemp.startRule.iRule
	}
	fmt.Printf("Lemon: nsymbol=%d nterminal=%d start=%q startRule=%d\n", lemp.nsymbol, lemp.nterminal, lemp.start, startRule)
	fmt.Printf("Rules:\n")
	for rp := lemp.rule; rp != nil; rp = rp.next {
		PrintRule(lemp, rp)
	}
	fmt.Printf("Symbols:\n")
	for i := 0; i < lemp.nsymbol; i++ {
		PrintSymbol(lemp, lemp.symbols[i])
	}
	if lemp.sorted != nil {
		fmt.Printf("States:\n")
		for i := 0; i < lemp.nstate; i++ {
			PrintState(lemp, lemp.sorted[i])
		}
	}
	if current != nil {
		printbasis()
	}
}

func Action_add_debug(pos int, stp *state, typ e_action, sp *symbol, rp *rule, stp2 *state) {
	iRule := ""
	if rp != nil {
		iRule = fmt.Sprintf(", rp=%d", rp.iRule)
	}
	stp2txt := ""
	if stp2 != nil {
		stp2txt = fmt.Sprintf(", stp2=%d", stp2.statenum)
	}
	fmt.Printf("Action_add(%d): state=%d, typ=%d, sp=%d%s%s\n", pos, stp.statenum, typ, sp.index, iRule, stp2txt)
}

func printplink(plp *plink) {
	for ; plp != nil; plp = plp.next {
		fmt.Printf(" %d.%d", plp.cfp.rp.iRule, plp.cfp.dot)
	}
	fmt.Printf("\n")
}

func printbasis() {
	fmt.Printf("basis:\n")
	for cp := current; cp != nil; cp = cp.next {
		fmt.Printf(" %d.%d status=%d", cp.rp.iRule, cp.dot, cp.status)
		if cp.next != nil {
			fmt.Printf(" next=%d.%d", cp.next.rp.iRule, cp.next.dot)
		}
		if cp.bp != nil {
			fmt.Printf(" bp=%d.%d", cp.bp.rp.iRule, cp.bp.dot)
		}
		if cp.stp != nil {
			fmt.Printf(" stp=%d", cp.stp.statenum)
		}
		fmt.Printf("\n")
		PrintSet(cp.fws, "  fws:")
		if cp.fplp != nil {
			fmt.Printf("  fplp:")
			printplink(cp.fplp)
		}
		if cp.bplp != nil {
			fmt.Printf("  bplp:")
			printplink(cp.bplp)
		}
	}
	fmt.Printf("\n")
}

// defines is a struct that holds what would be #defines in C. In Go,
// we don't have any such thing, so we'll use text replacements.
type defines struct {
	mappings map[string]string // the map of define to text replacement
	re       *regexp.Regexp    // the regular expression used to match the defines
}

// replaceAll replaces all known defines in a string with their mappings.
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

// buildRegexp builds a new regexp to match the current set of defines.
func (d *defines) buildRegexp() {
	keys := make([]string, 0, len(d.mappings))
	for key := range d.mappings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	d.re = regexp.MustCompile(`\b(` + strings.Join(keys, "|") + `)\b`)
}

// addDefine adds a single define. It also nils out the regexp, so
// that it will be rebuilt the next time it is needed.
func (d *defines) addDefine(define, replacement string) {
	if d.mappings == nil {
		d.mappings = make(map[string]string)
	}
	d.mappings[define] = replacement
	d.re = nil
}
