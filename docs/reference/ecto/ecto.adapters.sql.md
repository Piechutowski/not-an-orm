<!-- Ecto.Adapters.SQL: extracted verbatim from lib/ecto/adapters/sql.ex (ecto_sql repo) by fetch.sh. Apache-2.0. -->

# Ecto.Adapters.SQL


This application provides functionality for working with
SQL databases in `Ecto`.

## Built-in adapters

By default, we support the following adapters:

  * `Ecto.Adapters.Postgres` for Postgres
  * `Ecto.Adapters.MyXQL` for MySQL
  * `Ecto.Adapters.Tds` for SQLServer

## Additional functions

If your `Ecto.Repo` is backed by any of the SQL adapters above,
this module will inject additional functions into your repository:

  * `disconnect_all(interval, options \\ [])` -
     shortcut for `Ecto.Adapters.SQL.disconnect_all/3`

  * `explain(type, query, options \\ [])` -
     shortcut for `Ecto.Adapters.SQL.explain/4`

  * `query(sql, params, options \\ [])` -
     shortcut for `Ecto.Adapters.SQL.query/4`

  * `query!(sql, params, options \\ [])` -
     shortcut for `Ecto.Adapters.SQL.query!/4`

  * `query_many(sql, params, options \\ [])` -
     shortcut for `Ecto.Adapters.SQL.query_many/4`

  * `query_many!(sql, params, options \\ [])` -
     shortcut for `Ecto.Adapters.SQL.query_many!/4`

  * `to_sql(type, query)` -
     shortcut for `Ecto.Adapters.SQL.to_sql/3`

Generally speaking, you must invoke those functions directly from
your repository, for example: `MyApp.Repo.query("SELECT true")`.

You can also invoke them directly from `Ecto.Adapters.SQL`, but
keep in mind that in such cases the "dynamic repository" functionality
is not available by default. Instead, you must explicitly call
`YouRepo.get_dynamic_repo()` and pass it as first argument.

## Migrations

`ecto_sql` supports database migrations. You can generate a migration
with:

    $ mix ecto.gen.migration create_posts

This will create a new file inside `priv/repo/migrations` with the
`change` function. Check `Ecto.Migration` for more information.

To interface with migrations, developers typically use mix tasks:

  * `mix ecto.migrations` - lists all available migrations and their status
  * `mix ecto.migrate` - runs a migration
  * `mix ecto.rollback` - rolls back a previously run migration

If you want to run migrations programmatically, see `Ecto.Migrator`.

## SQL sandbox

`ecto_sql` provides a sandbox for testing. The sandbox wraps each
test in a transaction, making sure the tests are isolated and can
run concurrently. See `Ecto.Adapters.SQL.Sandbox` for more information.

## Structure load and dumping

If you have an existing database, you may want to dump its existing
structure and make it reproducible from within Ecto. This can be
achieved with two Mix tasks:

  * `mix ecto.load` - loads an existing structure into the database
  * `mix ecto.dump` - dumps the existing database structure to the filesystem

For creating and dropping databases, see `mix ecto.create`
and `mix ecto.drop` that are included as part of Ecto.

## Custom adapters

Developers can implement their own SQL adapters by using
`Ecto.Adapters.SQL` and by implementing the callbacks required
by `Ecto.Adapters.SQL.Connection`  for handling connections and
performing queries. The connection handling and pooling for SQL
adapters should be built using the `DBConnection` library.

When using `Ecto.Adapters.SQL`, the following options are required:

  * `:driver` (required) - the database driver library.
    For example: `:postgrex`


---

## `to_sql(kind, repo, queryable, opts \\ [])`

Converts the given query to SQL according to its kind and the
adapter in the given repository.

## Examples

The examples below are meant for reference. Each adapter will
return a different result:

    iex> MyRepo.to_sql(:all, Post)
    {"SELECT p.id, p.title, p.inserted_at, p.created_at FROM posts as p", []}

    iex> MyRepo.to_sql(:update_all, from(p in Post, update: [set: [title: ^"hello"]]))
    {"UPDATE posts AS p SET title = $1", ["hello"]}

    iex> Ecto.Adapters.SQL.to_sql(:all, MyRepo, Post)
    {"SELECT p.id, p.title, p.inserted_at, p.created_at FROM posts as p", []}


---

## `explain(repo, operation, queryable, opts \\ [])`

Executes an EXPLAIN statement or similar for the given query according to its kind and the
adapter in the given repository.

## Examples

    # Postgres
    iex> MyRepo.explain(:all, Post)
    "Seq Scan on posts p0  (cost=0.00..12.12 rows=1 width=443)"

    iex> Ecto.Adapters.SQL.explain(Repo, :all, Post)
    "Seq Scan on posts p0  (cost=0.00..12.12 rows=1 width=443)"

    # MySQL
    iex> MyRepo.explain(:all, from(p in Post, where: p.title == "title")) |> IO.puts()
    +----+-------------+-------+------------+------+---------------+------+---------+------+------+----------+-------------+
    | id | select_type | table | partitions | type | possible_keys | key  | key_len | ref  | rows | filtered | Extra       |
    +----+-------------+-------+------------+------+---------------+------+---------+------+------+----------+-------------+
    |  1 | SIMPLE      | p0    | NULL       | ALL  | NULL          | NULL | NULL    | NULL |    1 |    100.0 | Using where |
    +----+-------------+-------+------------+------+---------------+------+---------+------+------+----------+-------------+

    # Shared opts
    iex> MyRepo.explain(:all, Post, analyze: true, timeout: 20_000)
    "Seq Scan on posts p0  (cost=0.00..11.70 rows=170 width=443) (actual time=0.013..0.013 rows=0 loops=1)\\nPlanning Time: 0.031 ms\\nExecution Time: 0.021 ms"

It's safe to execute it for updates and deletes, no data change will be committed:

    iex> MyRepo.explain(Repo, :update_all, from(p in Post, update: [set: [title: "new title"]]))
    "Update on posts p0  (cost=0.00..11.70 rows=170 width=449)\\n  ->  Seq Scan on posts p0  (cost=0.00..11.70 rows=170 width=449)"

This function is also available under the repository with name `explain`:

    iex> MyRepo.explain(:all, from(p in Post, where: p.title == "title"))
    "Seq Scan on posts p0  (cost=0.00..12.12 rows=1 width=443)\\n  Filter: ((title)::text = 'title'::text)"

### Options

Built-in adapters support passing `opts` to the EXPLAIN statement according to the following:

Adapter          | Supported opts
---------------- | --------------
Postgrex         | `analyze`, `verbose`, `costs`, `settings`, `buffers`, `timing`, `summary`, `format`, `plan`, `wrap_in_transaction`
MyXQL            | `format`, `wrap_in_transaction`

All options except `format` are boolean valued and default to `false`.

The allowed `format` values are `:map`, `:yaml`, and `:text`:
  * `:map` is the deserialized JSON encoding.
  * `:yaml` and `:text` return the result as a string.

The built-in adapters support the following formats:
  * Postgrex: `:map`, `:yaml` and `:text`
  * MyXQL: `:map` and `:text`

The `wrap_in_transaction` option is a boolean that controls whether the command is run inside of a
transaction that is rolled back. This is useful when, for example, you'd like to use `analyze: true`
on an update or delete query without modifying data. Defaults to `true`.

The `:plan` option in Postgrex can take the values `:custom` or `:fallback_generic`. When `:custom`
is specified, the explain plan generated will consider the specific values of the query parameters
that are supplied. When using `:fallback_generic`, the specific values of the query parameters will
be ignored. `:fallback_generic` does not use PostgreSQL's built-in support for a generic explain
plan (available as of PostgreSQL 16), but instead uses a special implementation that works for PostgreSQL
versions 12 and above. Defaults to `:custom`.

Any other value passed to `opts` will be forwarded to the underlying adapter query function, including
shared Repo options such as `:timeout`. Non built-in adapters may have specific behaviour and you should
consult their documentation for more details.

For version compatibility, please check your database's documentation:

  * _Postgrex_: [PostgreSQL doc](https://www.postgresql.org/docs/current/sql-explain.html).
  * _MyXQL_: [MySQL doc](https://dev.mysql.com/doc/refman/8.0/en/explain.html).


---

## `disconnect_all(repo, interval, opts \\ [])`

Forces all connections in the repo pool to disconnect within the given interval.

Once this function is called, the pool will disconnect all of its connections
as they are checked in or as they are pinged. Checked in and idle connections
will be randomly disconnected within the given time interval.

If the connection has a backoff configured (which is the case by default),
disconnecting means an attempt at a new connection will be done immediately
after, without starting a new process for each connection. However, if backoff
has been disabled, the connection process will terminate. In such cases,
disconnecting all connections may cause the pool supervisor to restart
depending on the max_restarts/max_seconds configuration of the pool,
so you will want to set those carefully.

If you need connections to be restarted periodically, set the `:max_lifetime`
option in your repository configuration instead.


---

## `stream(repo, sql, params \\ [], opts \\ [])`

Returns a stream that runs a custom SQL query on given repo when reduced.

In case of success it is a enumerable containing maps with at least two keys:

  * `:num_rows` - the number of rows affected

  * `:rows` - the result set as a list. `nil` may be returned
    instead of the list if the command does not yield any row
    as result (but still yields the number of affected rows,
    like a `delete` command without returning would)

In case of failure it raises an exception.

If the adapter supports a collectable stream, the stream may also be used as
the collectable in `Enum.into/3`. Behaviour depends on the adapter.

## Options

  * `:log` - When false, does not log the query
  * `:max_rows` - The number of rows to load from the database as we stream

## Examples

    iex> Ecto.Adapters.SQL.stream(MyRepo, "SELECT $1::integer + $2", [40, 2]) |> Enum.to_list()
    [%{rows: [[42]], num_rows: 1}]


---

## `query!(repo, sql, params \\ [], opts \\ [])`

Same as `query/3` but returns result directly without `:ok` tuple
and raises on invalid queries


---

## `query(repo, sql, params \\ [], opts \\ [])`

Runs a custom SQL query.

If the query was successful, it will return an `:ok` tuple containing
a map with at least two keys:

  * `:num_rows` - the number of rows affected
  * `:rows` - the result set as a list. `nil` may be returned
    instead of the list if the command does not yield any row
    as result (but still yields the number of affected rows,
    like a `delete` command without returning would)

## Options

  * `:log` - When false, does not log the query
  * `:timeout` - Execute request timeout, accepts: `:infinity` (default: `#{@timeout}`);

## Examples

    iex> MyRepo.query("SELECT $1::integer + $2", [40, 2])
    {:ok, %{rows: [[42]], num_rows: 1}}

    iex> Ecto.Adapters.SQL.query(MyRepo, "SELECT $1::integer + $2", [40, 2])
    {:ok, %{rows: [[42]], num_rows: 1}}


---

## `query_many!(repo, sql, params \\ [], opts \\ [])`

Same as `query_many/4` but returns result directly without `:ok` tuple
and raises on invalid queries


---

## `query_many(repo, sql, params \\ [], opts \\ [])`

Runs a custom SQL query that returns multiple results on the given repo.

In case of success, it must return an `:ok` tuple containing a list of
maps with at least two keys:

  * `:num_rows` - the number of rows affected

  * `:rows` - the result set as a list. `nil` may be returned
    instead of the list if the command does not yield any row
    as result (but still yields the number of affected rows,
    like a `delete` command without returning would)

## Options

  * `:log` - When false, does not log the query
  * `:timeout` - Execute request timeout, accepts: `:infinity` (default: `#{@timeout}`);

## Examples

    iex> MyRepo.query_many("SELECT $1; SELECT $2;", [40, 2])
    {:ok, [%{rows: [[40]], num_rows: 1}, %{rows: [[2]], num_rows: 1}]}

    iex> Ecto.Adapters.SQL.query_many(MyRepo, "SELECT $1; SELECT $2;", [40, 2])
    {:ok, [%{rows: [[40]], num_rows: 1}, %{rows: [[2]], num_rows: 1}]}


---

## `table_exists?(repo, table, opts \\ [])`

Checks if the given `table` exists.

Returns `true` if the `table` exists in the `repo`, otherwise `false`.
The table is checked against the current database/schema in the connection.


---

## `first_non_ecto_stacktrace(stacktrace, %{repo: repo}, size)`

Receives a stacktrace, and return the first N items before Ecto entries

This function is used by default in the `:log_stacktrace_mfa` config, with
a size of 1.
