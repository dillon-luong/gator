-- name: CreateUser :one
insert into users (id, created_at, updated_at, name)
values (
    $1,
    $2,
    $3,
    $4
)
returning *;

-- name: GetUser :one
select * from users
where name = $1;

-- name: GetUserByID :one
select * from users
where id = $1;

-- name: GetUsers :many
select * from users;

-- name: DeleteUsers :exec
delete from users;