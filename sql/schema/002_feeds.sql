-- +goose Up
create table
    feeds (
        id UUID primary key,
        created_at timestamp not null,
        updated_at timestamp not null,
        name text not null,
        url text unique not null,
        user_id UUID not null references users(id) on delete cascade -- who added the feed initially
    );

-- +goose Down
drop table feeds;