-- name: CreateFeed :one
insert into feeds (id, created_at, updated_at, name, url, user_id)
values ($1, $2, $3, $4, $5, $6)
returning *;

-- name: GetFeeds :many
select * from feeds;

-- name: GetFeedByUrl :one
select * from feeds
where url = $1;

-- name: MarkFeedFetched :exec
update feeds
set last_fetched_at = $2, updated_at = $3
where id = $1;

-- name: GetNextFeedToFetch :one
select * from feeds
order by last_fetched_at asc nulls first
limit 1;
