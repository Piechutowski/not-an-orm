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

## `table_exists?(repo, table, opts \\ [])`

Checks if the given `table` exists.

Returns `true` if the `table` exists in the `repo`, otherwise `false`.
The table is checked against the current database/schema in the connection.


---

## `first_non_ecto_stacktrace(stacktrace, %{repo: repo}, size)`

Receives a stacktrace, and return the first N items before Ecto entries

This function is used by default in the `:log_stacktrace_mfa` config, with
a size of 1.
