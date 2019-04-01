drop table if exists bookmarks cascade;
drop table if exists accounts cascade;
drop table if exists weather cascade;
drop table if exists locations cascade;

create table locations
(
    id          serial       primary key,
    city_name   varchar(255) not null unique,
    query_count integer
);

create table weather
(
    location_id integer not null,
    labels      text[],
    temp_high   real,
    temp_low    real,
    at_time     timestamp not null
);

create table accounts
(
    id           serial primary key,
    user_name    varchar(255) not null unique
);

create table bookmarks
(
    id           integer primary key,
    location_ids integer[]
);
