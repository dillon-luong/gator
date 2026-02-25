-- name: CreateFeedFollow :one
with inserted_feed_follows as (
    insert into feed_follows (id, created_at, updated_at, user_id, feed_id)
    values ($1, $2, $3, $4, $5)
    returning *
)

select
    inserted_feed_follows.*,
    users.name as user_name,
    feeds.name as feed_name
from
    inserted_feed_follows
    inner join users on users.id = inserted_feed_follows.user_id
    inner join feeds on feeds.id = inserted_feed_follows.feed_id;

-- name: GetFeedFollowsForUser :many
select users.name as createdBy, feeds.name as feedName
from feed_follows
inner join feeds on feed_follows.feed_id = feeds.id
inner join users on feeds.user_id = users.id
where $1 = feed_follows.user_id;

-- name: DeleteFeedFollow :exec
delete from feed_follows
where user_id = $1 and feed_id = $2;