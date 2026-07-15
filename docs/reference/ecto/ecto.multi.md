<!-- Ecto.Multi: extracted verbatim from lib/ecto/multi.ex (ecto repo) by fetch.sh. Apache-2.0. -->

# Ecto.Multi


`Ecto.Multi` is a data structure for grouping multiple Repo operations.

`Ecto.Multi` makes it possible to pack operations that should be
performed in a single database transaction and provides a way to introspect
the queued operations without actually performing them. Each operation
is given a name that is unique and will identify its result in case of
either success or failure.

If a Multi is valid (i.e. all the changesets in it are valid),
all operations will be executed in the order they were added.

The `Ecto.Multi` structure should be considered opaque. You can use
`%Ecto.Multi{}` to pattern match the type, but accessing fields or
directly modifying them is not advised.

`Ecto.Multi.to_list/1` returns a canonical representation of the
structure that can be used for introspection.

> #### When to use Ecto.Multi? {: .info}
>
> `Ecto.Multi` is particularly useful when the set of operations to perform
> is dynamic. For most other use cases, using regular control flow within
> [`Repo.transact(fun)`](`c:Ecto.Repo.transact/2`) and returning
> `{:ok, result}` or `{:error, reason}` is more straightforward.

## Changesets

If a Multi contains operations that accept changesets (like `insert/4`,
`update/4` or `delete/4`), they will be checked before starting the
transaction. If any changeset has errors, the transaction will not be
started and the error will immediately be returned.

Note: `insert/4`, `update/4`, `insert_or_update/4` and `delete/4`
variants that accept a function do not perform these checks since
the functions are executed after the transaction has started.

## Run

`Multi` allows you to run arbitrary functions as part of your transaction
via `run/3` and `run/5`. This is especially useful when an operation
depends on the value of a previous operation. For this reason, the
function given as a callback to `run/3` and `run/5` will receive the repo
as the first argument, and all changes performed by the Multi so far as a
map as the second argument.

The function given to `run` must return `{:ok, value}` or `{:error, value}`
as its result. Returning an error will abort any further operations
and make the Multi fail.

## Example

Let's look at an example definition and usage: resetting a password. We need
to update the account with proper information, log the request and remove
all current sessions:

    defmodule PasswordManager do
      alias Ecto.Multi

      def reset(account, params) do
        Multi.new()
        |> Multi.update(:account, Account.password_reset_changeset(account, params))
        |> Multi.insert(:log, Log.password_reset_changeset(account, params))
        |> Multi.delete_all(:sessions, Ecto.assoc(account, :sessions))
      end
    end

We can later execute it in the integration layer using Repo:

    Repo.transact(PasswordManager.reset(account, params))

By pattern matching on the result we can differentiate various conditions:

    case result do
      {:ok, %{account: account, log: log, sessions: sessions}} ->
        # The Multi was successful. We can access results , which are as
        # we would get from running the corresponding Repo functions, under
        # keys we used for naming the operations.
      {:error, failed_operation, failed_value, changes_so_far} ->
        # One of the operations failed. We can access the operation's failure
        # value (such as a changeset for operations on changesets) to prepare a
        # proper response. We also get access to the results of any operations
        # that succeeded before the indicated operation failed. (However,
        # successful operations were rolled back.)
    end

We can also easily unit test our transaction without actually running it.
Since changesets can use in-memory data, we can use an account that is
constructed in memory as well, without persisting it to the database:

    test "dry run password reset" do
      account = %Account{password: "letmein"}
      multi = PasswordManager.reset(account, params)

      assert [
        {:account, {:update, account_changeset, []}},
        {:log, {:insert, log_changeset, []}},
        {:sessions, {:delete_all, query, []}}
      ] = Ecto.Multi.to_list(multi)

      # We can introspect changesets and query to see if everything
      # is as expected, for example:
      assert account_changeset.valid?
      assert log_changeset.valid?
      assert inspect(query) == "#Ecto.Query<from a in Session>"
    end

The name of each operation does not have to be an atom. This can be particularly
useful when you wish to update a collection of changesets at once, and track their
errors individually:

    accounts = [%Account{id: 1}, %Account{id: 2}]

    Enum.reduce(accounts, Multi.new(), fn account, multi ->
      Multi.update(
        multi,
        {:account, account.id},
        Account.password_reset_changeset(account, params)
      )
    end)


---

## `@type changes :: map`

Map of changes made so far during the current transaction. For any Multi
which returns `{:ok, result}`, its `t:name/0` is added as a key and its
result as the value.


---

## `@type name :: any`

Name of an operation in the Multi. Can be any term, as long as it is unique
within the list of operations; for example, `:insert_post` or `{:delete_post,
5}`.


---

## `@type failure :: {:error, failed_operation :: Ecto.Multi.name(), failed_value :: any(), changes_so_far :: %{Ec …`

Result of a failed transaction using a Multi.


---

## `new`

Returns an empty `Ecto.Multi` struct.

## Example

    iex> Ecto.Multi.new() |> Ecto.Multi.to_list()
    []


---

## `append(lhs, rhs)`

Appends the second Multi to the first.

All names must be unique within both structures.

## Example

    iex> lhs = Ecto.Multi.new() |> Ecto.Multi.run(:left, fn _, changes -> {:ok, changes} end)
    iex> rhs = Ecto.Multi.new() |> Ecto.Multi.run(:right, fn _, changes -> {:error, changes} end)
    iex> Ecto.Multi.append(lhs, rhs) |> Ecto.Multi.to_list |> Keyword.keys
    [:left, :right]


---

## `prepend(lhs, rhs)`

Prepends the second Multi to the first.

All names must be unique within both structures.

## Example

    iex> lhs = Ecto.Multi.new() |> Ecto.Multi.run(:left, fn _, changes -> {:ok, changes} end)
    iex> rhs = Ecto.Multi.new() |> Ecto.Multi.run(:right, fn _, changes -> {:error, changes} end)
    iex> Ecto.Multi.prepend(lhs, rhs) |> Ecto.Multi.to_list |> Keyword.keys
    [:right, :left]


---

## `merge(%Multi{} = multi, merge)`

Merges a Multi returned dynamically by an anonymous function.

This function is useful when the Multi to be merged requires information
from the original Multi. The second argument is an anonymous function
that receives the Multi changes so far. The anonymous function must return
another Multi.

If you would prefer to simply merge two Multis together, see `append/2` or
`prepend/2`.

Duplicated operations are not allowed.

## Example

    multi =
      Ecto.Multi.new()
      |> Ecto.Multi.insert(:post, %Post{title: "first"})

    multi
    |> Ecto.Multi.merge(fn %{post: post} ->
      Ecto.Multi.new()
      |> Ecto.Multi.insert(:comment, Ecto.build_assoc(post, :comments))
    end)
    |> MyApp.Repo.transact()


---

## `merge(%Multi{} = multi, mod, fun, args)`

Merges a Multi returned dynamically by calling `module` and `function` with `args`.

Similar to `merge/2` but allows passing of module name, function and
arguments. The function should return an `Ecto.Multi`, and receives changes so far
as the first argument (prepended to those passed in the call to the function).

Duplicated operations are not allowed.


---

## `insert(multi, name, changeset_or_struct_or_fun, opts \\ [])`

Adds an insert operation to the Multi.

The `name` must be unique within the Multi.

The remaining arguments and options are the same as in `c:Ecto.Repo.insert/2`.

## Example

    Ecto.Multi.new()
    |> Ecto.Multi.insert(:insert, %Post{title: "first"})
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.insert(:post, %Post{title: "first"})
    |> Ecto.Multi.insert(:comment, fn %{post: post} ->
      Ecto.build_assoc(post, :comments)
    end)
    |> MyApp.Repo.transact()


---

## `update(multi, name, changeset_or_fun, opts \\ [])`

Adds an update operation to the Multi.

The `name` must be unique within the Multi.

The remaining arguments and options are the same as in `c:Ecto.Repo.update/2`.

## Example

    post = MyApp.Repo.get!(Post, 1)
    changeset = Ecto.Changeset.change(post, title: "New title")
    Ecto.Multi.new()
    |> Ecto.Multi.update(:update, changeset)
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.insert(:post, %Post{title: "first"})
    |> Ecto.Multi.update(:fun, fn %{post: post} ->
      Ecto.Changeset.change(post, title: "New title")
    end)
    |> MyApp.Repo.transact()


---

## `insert_or_update(multi, name, changeset_or_fun, opts \\ [])`

Inserts or updates a changeset depending on whether or not the changeset was persisted.

The `name` must be unique within the Multi.

The remaining arguments and options are the same as in `c:Ecto.Repo.insert_or_update/2`.

## Example

    changeset = Post.changeset(%Post{}, %{title: "New title"})
    Ecto.Multi.new()
    |> Ecto.Multi.insert_or_update(:insert_or_update, changeset)
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.run(:post, fn repo, _changes ->
      {:ok, repo.get(Post, 1) || %Post{}}
    end)
    |> Ecto.Multi.insert_or_update(:update, fn %{post: post} ->
      Ecto.Changeset.change(post, title: "New title")
    end)
    |> MyApp.Repo.transact()


---

## `delete(multi, name, changeset_or_struct_fun, opts \\ [])`

Adds a delete operation to the Multi.

The `name` must be unique within the Multi.

The remaining arguments and options are the same as in `c:Ecto.Repo.delete/2`.

## Example

    post = MyApp.Repo.get!(Post, 1)
    Ecto.Multi.new()
    |> Ecto.Multi.delete(:delete, post)
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.run(:post, fn repo, _changes ->
      case repo.get(Post, 1) do
        nil -> {:error, :not_found}
        post -> {:ok, post}
      end
    end)
    |> Ecto.Multi.delete(:delete, fn %{post: post} ->
      # Others validations
      post
    end)
    |> MyApp.Repo.transact()


---

## `one(multi, name, queryable_or_fun, opts \\ [])`

Runs a query expecting one result and stores the result in the Multi.

The `name` must be unique within the Multi.

The remaining arguments and options are the same as in `c:Ecto.Repo.one/2`.

## Example

    Ecto.Multi.new()
    |> Ecto.Multi.one(:post, Post)
    |> Ecto.Multi.one(:author, fn %{post: post} ->
      from(a in Author, where: a.id == ^post.author_id)
    end)
    |> MyApp.Repo.transact()


---

## `all(multi, name, queryable_or_fun, opts \\ [])`

Runs a query and stores all results in the Multi.

The `name` must be unique within the Multi.

The remaining arguments and options are the same as in `c:Ecto.Repo.all/2`.

## Example

    Ecto.Multi.new()
    |> Ecto.Multi.all(:all, Post)
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.all(:all, fn _changes -> Post end)
    |> MyApp.Repo.transact()


---

## `exists?(multi, name, queryable_or_fun, opts \\ [])`

Checks if an entry matching the given query exists and stores a boolean in the Multi.

The `name` must be unique within the Multi.

The remaining arguments and options are the same as in `c:Ecto.Repo.exists?/2`.

## Example

    Ecto.Multi.new()
    |> Ecto.Multi.exists?(:post, Post)
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.exists?(:post, fn _changes -> Post end)
    |> MyApp.Repo.transact()


---

## `error(multi, name, value)`

Causes the Multi to fail with the given value.

Running the Multi in a transaction will execute
no previous steps and return the value of the first
error added.


---

## `run(multi, name, run)`

Adds a function to run as part of the Multi.

The function should return either `{:ok, value}` or `{:error, value}`,
and receives the repo as the first argument and the changes so far
as the second argument.

## Example

    Ecto.Multi.run(multi, :write, fn _repo, %{image: image} ->
      with :ok <- File.write(image.name, image.contents) do
        {:ok, nil}
      end
    end)


---

## `run(multi, name, mod, fun, args)`

Adds a function to run as part of the Multi.

Similar to `run/3`, but allows passing of module name, function and arguments.
The function should return either `{:ok, value}` or `{:error, value}`, and
receives the repo as the first argument and the changes so far as the
second argument (prepended to those passed in the call to the function).


---

## `insert_all(multi, name, schema_or_source, entries_or_query_or_fun, opts \\ [])`

Adds an `insert_all` operation to the Multi.

Accepts the same arguments and options as `c:Ecto.Repo.insert_all/3`.

## Example

    posts = [%{title: "My first post"}, %{title: "My second post"}]
    Ecto.Multi.new()
    |> Ecto.Multi.insert_all(:insert_all, Post, posts)
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.run(:post, fn repo, _changes ->
      case repo.get(Post, 1) do
        nil -> {:error, :not_found}
        post -> {:ok, post}
      end
    end)
    |> Ecto.Multi.insert_all(:insert_all, Comment, fn %{post: post} ->
      # Others validations

      entries
      |> Enum.map(fn comment ->
        Map.put(comment, :post_id, post.id)
      end)
    end)
    |> MyApp.Repo.transact()


---

## `update_all(multi, name, queryable_or_fun, updates, opts \\ [])`

Adds an `update_all` operation to the Multi.

Accepts the same arguments and options as `c:Ecto.Repo.update_all/3`.

## Example

    Ecto.Multi.new()
    |> Ecto.Multi.update_all(:update_all, Post, set: [title: "New title"])
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.run(:post, fn repo, _changes ->
      case repo.get(Post, 1) do
        nil -> {:error, :not_found}
        post -> {:ok, post}
      end
    end)
    |> Ecto.Multi.update_all(:update_all, fn %{post: post} ->
      # Others validations
      from(c in Comment, where: c.post_id == ^post.id, update: [set: [title: "New title"]])
    end, [])
    |> MyApp.Repo.transact()


---

## `delete_all(multi, name, queryable_or_fun, opts \\ [])`

Adds a `delete_all` operation to the Multi.

Accepts the same arguments and options as `c:Ecto.Repo.delete_all/2`.

## Example

    queryable = from(p in Post, where: p.id < 5)
    Ecto.Multi.new()
    |> Ecto.Multi.delete_all(:delete_all, queryable)
    |> MyApp.Repo.transact()

    Ecto.Multi.new()
    |> Ecto.Multi.run(:post, fn repo, _changes ->
      case repo.get(Post, 1) do
        nil -> {:error, :not_found}
        post -> {:ok, post}
      end
    end)
    |> Ecto.Multi.delete_all(:delete_all, fn %{post: post} ->
      # Others validations
      from(c in Comment, where: c.post_id == ^post.id)
    end)
    |> MyApp.Repo.transact()


---

## `to_list(%Multi{operations: operations})`

Returns the list of operations stored in the Multi.

Always use this function when you need to access the operations you
have defined in `Ecto.Multi`. Inspecting the `Ecto.Multi` struct internals
directly is discouraged.


---

## `put(multi, name, value)`

Adds a value to the changes so far under the given name.

The given `value` is added to the Multi before the transaction starts.
If you would like to run arbitrary functions as part of your transaction,
see `run/3` or `run/5`.

## Example

Imagine there is an existing company schema that you retrieved from
the database. You can insert it as a change in the Multi using `put/3`:

    Ecto.Multi.new()
    |> Ecto.Multi.put(:company, company)
    |> Ecto.Multi.insert(:user, fn changes -> User.changeset(changes.company) end)
    |> Ecto.Multi.insert(:person, fn changes -> Person.changeset(changes.user, changes.company) end)
    |> MyApp.Repo.transact()

In the example above, there isn't a significant benefit in putting
the `company` in the Multi because you could also access the
`company` variable directly inside the anonymous function.

However, the benefit of `put/3` is seen when composing `Ecto.Multi`s.
If the insert operations above were defined in another module,
you could use `put(:company, company)` to inject changes that
will be accessed by other functions down the chain, removing
the need to pass both `multi` and `company` values around.


---

## `inspect(multi, opts \\ [])`

Inspects results from a Multi.

By default, the name is shown as a label to the inspect. Custom labels are
supported through the `IO.inspect/2` `label` option.

## Options

All options for IO.inspect/2 are supported, as well as:

  * `:only` - A field or a list of fields to inspect, will print the entire
    map by default.

## Examples

    Ecto.Multi.new()
    |> Ecto.Multi.insert(:person_a, changeset)
    |> Ecto.Multi.insert(:person_b, changeset)
    |> Ecto.Multi.inspect()
    |> MyApp.Repo.transact()

Prints:
    %{person_a: %Person{...}, person_b: %Person{...}}

We can use the `:only` option to limit which fields will be printed:

    Ecto.Multi.new()
    |> Ecto.Multi.insert(:person_a, changeset)
    |> Ecto.Multi.insert(:person_b, changeset)
    |> Ecto.Multi.inspect(only: :person_a)
    |> MyApp.Repo.transact()

Prints:
    %{person_a: %Person{...}}
