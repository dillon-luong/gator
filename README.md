REQS:
need Postgres and Go installed to run/install this program with `go run`

TOOLS:
This uses SQLC to gen db queries and Goose for db migrations

DESC:
CLI tool that lets you add users, follow RSS feeds, and aggregate and view the 
posts of those fields

SETUP INSTRUCTIONS:
can install with `go install github.com/dillon-luong/gatorcli@latest` or 
cloning it locally and installing it with `go install` from module root
 
This fetches the module from the public/local repo, compiles it and drops the 
`gator` executable into `$(go env GOPATH)/bin` (or`$GOBIN` if you’ve set that).  
After that `gator` is on the command line and the Go toolchain is no longer 
needed.

For creating your Postgres db, 
run `goose postgres postgres://[user]:[password]@[host]:5432/gator up`
from the sql/schema directory

SETUP CONFIG:
before running the program, you'll need to setup a config file.
It should be located in your home directory as ~/.gatorconfig.json
It should contain:
{"db_url":"[your db connection string]?sslmode=disable"}

RUNNING THE PROGRAM:
There's a handful of commands you can run. They're listed in main.go under 
the main func.
The general order to use them is:
create/login a user: register [username], login [username]
add a feed (or follow an existing one, must have unique url): addfeed "feed name" "feed url", follow "feed url"
aggregate the posts the feeds the currently logged in user is following to db: agg [duration, e.g. 30s, 1m, time between agg each feed]
look at the posts you've stored up: browse [optional: # of posts, auto sorted by most recent]

some other useful commands:
users, feeds: see all registered users/feeds
unfollow: unfollow a feed from current user via url
following: see what feeds the current user is following