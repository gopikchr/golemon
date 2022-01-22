// A test case for the LEMON parser generator.  Run as follows:
//
//     lemongo lemon-test01.y && go run ./lemon-test01.go
//

%token_prefix TK_
%token_type   int
%default_type int
%include {
var (
	nSyntaxError = 0
	nAccept      = 0
	nFailure     = 0
)

func yytestcase(condition bool) {}
}

all ::=  A B.
all ::=  error B.

%syntax_error {
	nSyntaxError++
}
%parse_accept {
	nAccept++
}
%parse_failure {
	nFailure++
}
%code {
var nTest int
var nErr int

func testCase(testId int, shouldBe int, actual int, name string) {
	nTest++
	if shouldBe == actual {
		fmt.Printf("test %d: ok\n", testId)
	} else {
		fmt.Printf("test %d: got %d, expected %d\n", testId, actual, shouldBe)
		nErr++
	}
}

func main() {
	var xp yyParser
	xp.ParseInit()
	xp.Parse(TK_A, 0)
	xp.Parse(TK_B, 0)
	xp.Parse(0, 0)
	xp.ParseFinalize()
	testCase(100, 0, nSyntaxError, "nSyntaxError")
	testCase(110, 1, nAccept, "nAccept")
	testCase(120, 0, nFailure, "nFailure")
	nSyntaxError = 0
	nAccept = 0
	nFailure = 0
	xp.ParseInit()
	xp.Parse(TK_B, 0)
	xp.Parse(TK_B, 0)
	xp.Parse(0, 0)
	xp.ParseFinalize()
	testCase(200, 1, nSyntaxError, "nSyntaxError")
	testCase(210, 1, nAccept, "nAccept")
	testCase(220, 0, nFailure, "nFailure")
	nSyntaxError = 0
	nAccept = 0
	nFailure = 0
	xp.ParseInit()
	xp.Parse(TK_A, 0)
	xp.Parse(TK_A, 0)
	xp.Parse(0, 0)
	xp.ParseFinalize()
	testCase(300, 1, nSyntaxError, "nSyntaxError")
	testCase(310, 0, nAccept, "nAccept")
	testCase(320, 0, nFailure, "nFailure")
	if nErr == 0 {
		fmt.Printf("%d tests pass\n", nTest)
	} else {
		fmt.Printf("%d errors out %d tests\n", nErr, nTest)
		os.Exit(nErr)
	}
}
}
