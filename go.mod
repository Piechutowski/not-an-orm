module github.com/Piechutowski/not-an-orm

go 1.24

require github.com/urfave/cli/v3 v3.10.1

// mattn/go-sqlite3 is a TEST-ONLY dependency (itest, D25): generated code
// and the rt runtime import stdlib + this module only (D03).
require github.com/mattn/go-sqlite3 v1.14.47
