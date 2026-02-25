package main

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/dillon-luong/gatorcli/internal/config"
	"github.com/dillon-luong/gatorcli/internal/database"
	"github.com/google/uuid"
	pq "github.com/lib/pq"
)

type state struct {
	cfg *config.Config
	db  *database.Queries
}

type command struct {
	name string
	args []string
}

type commands struct {
	list map[string]func(*state, command) error
}

func (c *commands) run(s *state, cmd command) error {
	cmdToRun, ok := c.list[cmd.name]
	if !ok {
		return fmt.Errorf("Command not found")
	}

	if err := cmdToRun(s, cmd); err != nil {
		return fmt.Errorf("Error running command: %v", err)
	}

	return nil
}

func (c *commands) register(name string, f func(*state, command) error) {
	c.list[name] = f
}

type RSSFeed struct {
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Item        []RSSItem `xml:"item"`
	} `xml:"channel"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func main() {
	// set up state struct
	cfg := config.Read()
	db, err := sql.Open("postgres", cfg.Db_Url)
	if err != nil {
		log.Fatalf("Error retrieving db url: %v", err)
	}
	dbQueries := database.New(db)

	s := state{
		&cfg,
		dbQueries,
	}

	// set up cli commands
	commands := commands{
		make(map[string]func(*state, command) error),
	}
	commands.register("login", handlerLogin)
	commands.register("register", handlerRegister)
	commands.register("reset", handlerReset)
	commands.register("users", handlerListUsers)
	commands.register("agg", handlerAgg)
	commands.register("addfeed", middlewareLoggedIn(handlerAddFeed))
	commands.register("feeds", handlerListFeeds)
	commands.register("follow", middlewareLoggedIn(handlerFollow))
	commands.register("following", middlewareLoggedIn(handlerFollowing))
	commands.register("unfollow", middlewareLoggedIn(handlerUnfollow))
	commands.register("browse", middlewareLoggedIn(handlerBrowse))

	// retrieve actual command + args
	args := os.Args // format: [program name, command name, ...additional args]
	if len(args) < 2 {
		log.Fatal("No command listed")
	}
	var actualArgs []string
	if len(args) >= 3 {
		actualArgs = args[2:]
	}
	cmd := command{
		name: args[1],
		args: actualArgs,
	}

	// run command with args
	if err := commands.run(&s, cmd); err != nil {
		log.Fatal(err)
	}
}

// sets current user
// f: login [name]
func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("No username specified")
	}

	// make sure user exists in db
	_, err := s.db.GetUser(context.Background(), cmd.args[0])
	if err != nil {
		return fmt.Errorf("Error retrieving user: %v", err)
	}

	err = s.cfg.SetUser(cmd.args[0])
	if err != nil {
		return fmt.Errorf("error setting user in config: %v", err)
	}

	fmt.Println("User has been updated")
	return nil
}

// create new user in db, set user as curr user
// f: register [name]
func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("No name specified")
	}

	name := cmd.args[0]
	// ctx background is an empty ctx arg
	_, err := s.db.CreateUser(context.Background(), database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Name:      name,
	})
	// will error if user already exists (name has unique constraint)
	if err != nil {
		return fmt.Errorf("Error registering user: %v", err)
	}

	// set curr user
	err = s.cfg.SetUser(name)
	if err != nil {
		return fmt.Errorf("error setting user in config: %v", err)
	}

	fmt.Printf("new user was created and set as curr user: %v\n", name)
	return nil
}

// deletes all users from user table, for unit testing
// also deletes feeds and feed_follows since they have on delete cascade
// NOTE: not a good idea for prod dbs, but in this small proj its useful
// f: reset
func handlerReset(s *state, cmd command) error {
	err := s.db.DeleteUsers(context.Background())
	if err != nil {
		return fmt.Errorf("Error attempting to delete all users: %v", err)
	}

	return nil
}

// lists users from db and indicates which is the curr user
// f: users
func handlerListUsers(s *state, cmd command) error {
	users, err := s.db.GetUsers(context.Background())
	if err != nil {
		return fmt.Errorf("Error retrieving all users: %v", err)
	}

	curr_user := s.cfg.Current_User_Name
	for _, elem := range users {
		if curr_user == elem.Name {
			fmt.Printf("* %v (current)\n", elem.Name)
		} else {
			fmt.Printf("* %v\n", elem.Name)
		}
	}

	return nil
}

// aggregates rss available feeds
// intended use case is to run infinitely in background
// f: agg
func handlerAgg(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("Need at least one arg (time duration)")
	}

	time_between_reqs, err := time.ParseDuration(cmd.args[0])
	if err != nil {
		return fmt.Errorf("Error parsing time between reqs: %v", err)
	}
	fmt.Printf("Collecting feed every %v\n", time_between_reqs)

	ticker := time.NewTicker(time_between_reqs)
	for range ticker.C {
		if err := scrapeFeed(s); err != nil {
			return fmt.Errorf("Error scraping feed: %v", err)
		}
	}

	return nil
}

func scrapeFeed(s *state) error {
	// get next feed to fetch (not fetched/oldest)
	feed, err := s.db.GetNextFeedToFetch(context.Background())
	if err != nil {
		return fmt.Errorf("Error getting next feed to fetch: %v", err)
	}

	// mark feed as fetched
	err = s.db.MarkFeedFetched(context.Background(), database.MarkFeedFetchedParams{
		ID:            feed.ID,
		LastFetchedAt: sql.NullTime{Time: time.Now(), Valid: true},
		UpdatedAt:     time.Now(),
	})

	// fetch feed
	rssFeed, err := fetchFeed(context.Background(), feed.Url)
	if err != nil {
		return fmt.Errorf("Error calling Agg function: %v", err)
	}

	// parse html escape entities and save
	fmt.Printf("Title: %v\n", html.UnescapeString(rssFeed.Channel.Title))
	fmt.Printf("Link: %v\n", rssFeed.Channel.Link)
	fmt.Printf("Desc: %v\n", html.UnescapeString(rssFeed.Channel.Description))
	fmt.Println()
	for _, elem := range rssFeed.Channel.Item {
		title := html.UnescapeString(elem.Title)
		url := elem.Link
		desc := html.UnescapeString(elem.Description)
		// attempts to parse into layout RFC1123Z, e.g. Mon, 02 Jan 2006 15:04:05 -0700
		pubDate, err := time.Parse(time.RFC1123Z, elem.PubDate)
		// if doesn't parse, then just set to time.Now()
		if err != nil {
			pubDate = time.Now()
		}
		_, err = s.db.CreatePost(context.Background(), database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Title:       sql.NullString{String: title, Valid: title != ""},
			Url:         sql.NullString{String: url, Valid: url != ""},
			Description: sql.NullString{String: desc, Valid: desc != ""},
			PublishedAt: sql.NullTime{Time: pubDate, Valid: !pubDate.IsZero()}, // is this necessary? prob not
			FeedID:      feed.ID,
		})
		if err != nil {
			if pqErr, ok := err.(*pq.Error); ok {
				if pqErr.Code == "23505" { // error code for non-unique url?
					// return fmt.Errorf("a feed with that URL already exists")
					fmt.Println("found existing url, skipping")
				}
			} else {
				fmt.Printf("Error creating post %v: %v\n", title, err)
			}
		}
	}

	return nil
}

// helper func to get rss feed from specified url
func fetchFeed(ctx context.Context, feedURL string) (*RSSFeed, error) {
	// setup request
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating request for url %v: %v", feedURL, err)
	}
	req.Header.Set("User-Agent", "gator")

	// setup client and make request
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error creating client for request: %v", err)
	}
	defer res.Body.Close()

	// read xml
	xmlData, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading http response: %v", err)
	}

	// parse xml into struct
	var rssFeed RSSFeed
	err = xml.Unmarshal(xmlData, &rssFeed)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling response: %v", err)
	}

	return &rssFeed, nil
}

// creates feed in db, sets user as curr_user; assumes feed doesn't already exist
// f: agg [feed name, feed url]
func handlerAddFeed(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 2 {
		return fmt.Errorf("Not enough args. Two required: feed name, feed url")
	}
	name := cmd.args[0]
	url := cmd.args[1]

	feed, err := s.db.CreateFeed(context.Background(), database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Name:      name,
		Url:       url,
		UserID:    user.ID,
	})
	if err != nil {
		return fmt.Errorf("Error creating feed: %v", err)
	}

	_, err = s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return fmt.Errorf("Error creating feed follows entry: %v", err)
	}

	fmt.Printf("%+v", feed)
	return nil
}

// lists feeds from db
// f: feeds
func handlerListFeeds(s *state, cmd command) error {
	feeds, err := s.db.GetFeeds(context.Background())
	if err != nil {
		return fmt.Errorf("Error retrieving all feeds: %v", err)
	}

	for _, elem := range feeds {
		fmt.Printf("%v\n", elem.Name)
		fmt.Printf("* url: %v\n", elem.Url)
		user, err := s.db.GetUserByID(context.Background(), elem.UserID)
		if err != nil {
			return fmt.Errorf("Error getting user for feed: %v", err)
		}
		fmt.Printf("* created by: %v\n", user.Name)
	}

	return nil
}

// has curr_user follow specified feed by (assumed to be) existing url
// f: follow [url]
func handlerFollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("No url specified")
	}

	url := cmd.args[0]
	feed, err := s.db.GetFeedByUrl(context.Background(), url)
	if err != nil {
		return fmt.Errorf("Error getting feed by url: %v", err)
	}

	feedFollow, err := s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return fmt.Errorf("Error creating feedFollow entry: %v", err)
	}

	fmt.Printf("curr user %v is now following %v feed\n", feedFollow.UserName, feedFollow.FeedName)

	return nil
}

// lists feeds that curr user is following
// f: following
func handlerFollowing(s *state, cmd command, user database.User) error {
	feeds, err := s.db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		return fmt.Errorf("Error getting feeds followed by curr user: %v", err)
	}

	fmt.Printf("%v is following:\n", user.Name)
	for _, elem := range feeds {
		fmt.Printf("* %v\n", elem.Feedname)
	}

	return nil
}

// wrapper that takes in "Logged In" handler with user arg
func middlewareLoggedIn(handler func(s *state, cmd command, user database.User) error) func(*state, command) error {
	return func(s *state, cmd command) error {
		user, err := s.db.GetUser(context.Background(), s.cfg.Current_User_Name)
		if err != nil {
			return fmt.Errorf("Error retrieving curr user: %v", err)
		}

		return handler(s, cmd, user)
	}
}

// unfollows a feed that curr user is following
// f: unfollow [url]
func handlerUnfollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("No args specified, requires feed url arg")
	}

	url := cmd.args[0]
	feed, err := s.db.GetFeedByUrl(context.Background(), url)
	if err != nil {
		return fmt.Errorf("Error getting feed by url: %v", err)
	}

	s.db.DeleteFeedFollow(context.Background(), database.DeleteFeedFollowParams{
		UserID: user.ID,
		FeedID: feed.ID,
	})

	return nil
}

// prints out [limit] number of posts from feeds that the user follows, with most recent first
// f: browse [optional: limit]
func handlerBrowse(s *state, cmd command, user database.User) error {
	var limit int
	if len(cmd.args) > 0 {
		var err error
		limit, err = strconv.Atoi(cmd.args[0])
		if err != nil {
			return fmt.Errorf("Error parsing limit: %v", err)
		}
	} else {
		limit = 2
	}

	posts, err := s.db.GetPostsForUser(context.Background(), database.GetPostsForUserParams{
		UserID: user.ID,
		Limit: int32(limit),
	})
	if err != nil {
		return fmt.Errorf("Error retrieving posts for user: %v", err)
	}

	for _, elem := range posts {
		fmt.Printf("Title: %v\n", elem.Title.String)
		fmt.Printf("* Link: %v\n", elem.Url.String)
		fmt.Printf("* Desc: %v\n", elem.Description.String)
		fmt.Printf("* Pub Date: %v\n", elem.PublishedAt.Time)
		fmt.Println()
	}

	return nil
}