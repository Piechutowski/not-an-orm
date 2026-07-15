<!-- Ecto.Changeset: extracted verbatim from lib/ecto/changeset.ex (ecto repo) by fetch.sh. Apache-2.0. -->

# Ecto.Changeset


Changesets allow filtering, type casting, validation, and
constraints when manipulating structs, usually in preparation
for inserting and updating entries into a database.

Let's break down what those features mean. Imagine the common
scenario where you want to receive data from a user submitted
web form to create or update entries in the database. Once you
receive this data on the server, changesets will help you perform
the following actions:

  * **filtering** - because you are receiving external data from
    a third-party, you must explicitly list which data you accept.
    For example, you most likely don't want to allow a user to set
    its own "is_admin" field to true

  * **type casting** - a web form sends most of its data as strings.
    When the user types the number "100", Ecto will receive it as
    the string "100", which must then be converted to 100.
    Changesets are responsible for converting these values to the
    types defined in your `Ecto.Schema`, support even complex types
    such as datetimes

  * **validations** - the data the user submits may not correct.
    For example, the user may type an invalid email address, with
    a trailing dot. Or say the date for a future meeting would
    happen in the last year. You must validate the data and give
    feedback to the user

  * **constraints** - some validations can only happen with the
    help of the database. For example, in order to know if a user
    email is already taken or not, you must query the database.
    Constraints help you do that in a way that respects data
    integrity

Although we have used a web form as an example, changesets can be used
for APIs and many other scenarios. Changesets may also be used to work
with data even if it won't be written to a database. We will cover
these scenarios in the documentation below. There is also an introductory
example of working with changesets and how it relates to schemas and
repositories [in the `Ecto` module](`Ecto#module-changesets`).

In a nutshell, there are two main functions for creating a changeset.
The `cast/4` function is used to receive external parameters from a
form, API or command line, and convert them to the types defined in
your `Ecto.Schema`. `change/2` is used to modify data directly from
your application, assuming the data given is valid and matches the
existing types. The remaining functions in this module, such as
validations, constraints, association handling, are about manipulating
changesets.

## External vs internal data

Changesets allow working with two kinds of data:

  * external to the application - for example user input from
    a form that needs to be type-converted and properly validated. This
    use case is primarily covered by the `cast/4` function.

  * internal to the application - for example programmatically generated,
    or coming from other subsystems. This use case is primarily covered
    by the `change/2` and `put_change/3` functions.

When working with external data, the data is typically provided
as maps with string keys (also known as parameters). On the other hand,
when working with internal data, you typically have maps of atom keys
or structs. This duality allows you to track the nature of your data:
if you have structs or maps with atom keys, it means the data has been
parsed/validated.

If you have external data or you have maps that may have either
string or atom keys, consider using `cast/4` to create a changeset.
The changeset will parse and validate these parameters and provide APIs
to safely manipulate and change the data accordingly.

## Validations and constraints

Ecto changesets provide both validations and constraints which
are ultimately turned into errors in case something goes wrong.

The difference between them is that most validations can be
executed without a need to interact with the database and, therefore,
are always executed before attempting to insert or update the entry
in the database. Validations run immediately when a validation function
is called on the data that is contained in the changeset at that time.

Some validations may happen against the database but
they are inherently unsafe. Those validations start with a `unsafe_`
prefix, such as `unsafe_validate_unique/4`.

On the other hand, constraints rely on the database and are always safe.
As a consequence, validations are always checked before constraints.
Constraints won't even be checked in case validations failed.

Let's see an example:

    defmodule User do
      use Ecto.Schema
      import Ecto.Changeset

      schema "users" do
        field :name
        field :email
        field :age, :integer
      end

      def changeset(user, params \\ %{}) do
        user
        |> cast(params, [:name, :email, :age])
        |> validate_required([:name, :email])
        |> validate_format(:email, ~r/@/)
        |> validate_inclusion(:age, 18..100)
        |> unique_constraint(:email)
      end
    end

In the `changeset/2` function above, we define three validations.
They check that `name` and `email` fields are present in the
changeset, the e-mail is of the specified format, and the age is
between 18 and 100 - as well as a unique constraint in the email
field.

Let's suppose the e-mail is given but the age is invalid. The
changeset would have the following errors:

    changeset = User.changeset(%User{}, %{age: 0, email: "mary@example.com"})
    {:error, changeset} = Repo.insert(changeset)
    changeset.errors #=> [age: {"is invalid", []}, name: {"can't be blank", []}]

In this case, we haven't checked the unique constraint in the
e-mail field because the data did not validate. Let's fix the
age and the name, and assume that the e-mail already exists in the
database:

    changeset = User.changeset(%User{}, %{age: 42, name: "Mary", email: "mary@example.com"})
    {:error, changeset} = Repo.insert(changeset)
    changeset.errors #=> [email: {"has already been taken", []}]

Validations and constraints define an explicit boundary when the check
happens. By moving constraints to the database, we also provide a safe,
correct and data-race free means of checking the user input.

### Deferred constraints

Some databases support deferred constraints, i.e., constraints which are
checked at the end of the transaction rather than at the end of each statement.

Changesets do not support this type of constraints. When working with deferred
constraints, a violation while invoking `c:Ecto.Repo.insert/2` or `c:Ecto.Repo.update/2` won't
return `{:error, changeset}`, but rather raise an error at the end of the
transaction.

## Trimming and empty values

Many times, the data given on cast needs to be further pruned, specially
regarding empty values. For example, if you are gathering data to be
cast from the command line or through an HTML form or any other text-based
format, it is likely those means cannot express nil values. For
those reasons, changesets include the ability of trimming values through
the `:trim_values` function.

When applying changes using `cast/4`, values are automatically trimmed
and then checked if they are empty by checking if they belong to the
`changeset.empty_values` field (which defaults to `[""]`).
If they are empty, then the field is set to its default value.
If the field is an array type, any empty value inside the array will
be removed. You can set the default empty values with `:empty_values`
option or directly in your changeset.

## Associations, embeds, and on replace

Using changesets you can work with associations as well as with
[embedded](embedded-schemas.html) structs. There are two primary APIs:

  * `cast_assoc/3` and `cast_embed/3` - those functions are used when
    working with external data. In particular, they allow you to change
    associations and embeds alongside the parent struct, all at once.

  * `put_assoc/4` and `put_embed/4` - it allows you to replace the
    association or embed as a whole. This can be used to move associated
    data from one entry to another, to completely remove or replace
    existing entries.

These functions are opinionated on how it works with associations.
If you need different behaviour or explicit control over the associated
data, you can skip this functionality and use `Ecto.Multi` to encode how
several database operations will happen on several schemas and changesets
at once.

You can learn more about working with associations in our documentation,
including cheatsheets and practical examples. Check out:

  * The docs for `cast_assoc/3` and `put_assoc/3`
  * The [associations cheatsheet](associations.html)
  * The [Constraints and Upserts guide](constraints-and-upserts.html)
  * The [Polymorphic associations with many to many guide](polymorphic-associations-with-many-to-many.html)

### The `:on_replace` option

When using any of those APIs, you may run into situations where Ecto sees
data is being replaced. For example, imagine a Post has many Comments where
the comments have IDs 1, 2 and 3. If you call `cast_assoc/3` passing only
the IDs 1 and 2, Ecto will consider 3 is being "replaced" and it will raise
by default. Such behaviour can be changed when defining the relation by
setting `:on_replace` option when defining your association/embed according
to the values below:

  * `:raise` (default) - do not allow removing association or embedded
    data via parent changesets
  * `:mark_as_invalid` - if attempting to remove the association or
    embedded data via parent changeset - an error will be added to the parent
    changeset, and it will be marked as invalid
  * `:nilify` - sets owner reference column to `nil` (available only for
    associations). Use this on a `belongs_to` column to allow the association
    to be cleared out so that it can be set to a new value. Will set `action`
    on associated changesets to `:replace`
  * `:update` - updates the association, available only for `has_one`, `belongs_to`
    and `embeds_one`. This option will update all the fields given to the changeset
    including the id for the association
  * `:delete` - removes the association or related data from the database.
    This option has to be used carefully (see below). Will set `action` on associated
    changesets to `:replace`
  * `:delete_if_exists` - like `:delete` except that it ignores any stale entry
    error. For instance, if you set `on_replace: :delete` but the replaced
    resource was already deleted by a separate request, it will raise a
    `Ecto.StaleEntryError`. `:delete_if_exists` makes it so it will only delete
    if the entry still exists

The `:delete` and `:delete_if_exists` options must be used carefully as they allow
users to delete any associated data by simply setting it to `nil` or an empty list.
If you need deletion, it is often preferred to add a separate boolean virtual field
in the schema and manually mark the changeset for deletion if the `:delete` field is
set in the params, as in the example below. Note that we don't call `cast/4` in this
case because we don't want to prevent deletion if a change is invalid (changes are
irrelevant if the entity needs to be deleted).

    defmodule Comment do
      use Ecto.Schema
      import Ecto.Changeset

      schema "comments" do
        field :body, :string
        field :delete, :boolean, virtual: true
      end

      def changeset(comment, %{"delete" => "true"}) do
        %{Ecto.Changeset.change(comment, delete: true) | action: :delete}
      end

      def changeset(comment, params) do
        cast(comment, params, [:body])
      end
    end

## Schemaless changesets

In the changeset examples so far, we have always used changesets to validate
and cast data contained in a struct defined by an Ecto schema, such as the `%User{}`
struct defined by the `User` module.

However, changesets can also be used with "regular" structs too by passing a tuple
with the data and its types:

    user = %User{}
    types = %{name: :string, email: :string, age: :integer}
    params = %{name: "Callum", email: "callum@example.com", age: 27}
    changeset =
      {user, types}
      |> Ecto.Changeset.cast(params, Map.keys(types))
      |> Ecto.Changeset.validate_required(...)
      |> Ecto.Changeset.validate_length(...)

where the user struct refers to the definition in the following module:

    defmodule User do
      defstruct [:name, :email, :age]
    end

Changesets can also be used with data in a plain map, by following the same API:

    data  = %{}
    types = %{name: :string, email: :string, age: :integer}
    params = %{name: "Callum", email: "callum@example.com", age: 27}
    changeset =
      {data, types}
      |> Ecto.Changeset.cast(params, Map.keys(types))
      |> Ecto.Changeset.validate_required(...)
      |> Ecto.Changeset.validate_length(...)

Besides the basic types which are mentioned above, such as `:boolean` and `:string`,
parameterized types can also be used in schemaless changesets. They implement
the `Ecto.ParameterizedType` behaviour and we can create the necessary type info by
calling the `init/2` function.

For example, to use `Ecto.Enum` in a schemaless changeset:

    types = %{
      name: :string,
      role: Ecto.ParameterizedType.init(Ecto.Enum, values: [:reader, :editor, :admin])
    }

    data  = %{}
    params = %{name: "Callum", role: "reader"}

    changeset =
      {data, types}
      |> Ecto.Changeset.cast(params, Map.keys(types))
      |> Ecto.Changeset.validate_required(...)
      |> Ecto.Changeset.validate_length(...)

Schemaless changesets make Ecto extremely useful to cast, validate and prune data even
if it is not meant to be persisted to the database.

### Changeset actions

Changesets have an action field which is usually set by `Ecto.Repo`
whenever one of the operations such as `insert` or `update` is called:

    changeset = User.changeset(%User{}, %{age: 42, email: "mary@example.com"})
    {:error, changeset} = Repo.insert(changeset)
    changeset.action
    #=> :insert

This means that when working with changesets that are not meant to be
persisted to the database, such as schemaless changesets, you may need
to explicitly set the action to one specific value. Frameworks such as
Phoenix [use the action value to define how HTML forms should
act](https://hexdocs.pm/phoenix_live_view/Phoenix.Component.html#form/1-a-note-on-errors).

Instead of setting the action manually, you may use `apply_action/2` that
emulates operations such as `c:Ecto.Repo.insert`. `apply_action/2` will return
`{:ok, changes}` if the changeset is valid or `{:error, changeset}`, with
the given `action` set in the changeset in case of errors.

## The Ecto.Changeset struct

The public fields are:

  * `valid?`       - Stores if the changeset is valid
  * `data`         - The changeset source data, for example, a struct
  * `params`       - The parameters as given on changeset creation
  * `changes`      - The `changes` from parameters that were approved in casting
  * `errors`       - All errors from validations
  * `required`     - All required fields as a list of atoms
  * `action`       - The action to be performed with the changeset
  * `types`        - Cache of the data's field types
  * `empty_values` - A list of values to be considered empty
  * `repo`         - The repository applying the changeset (only set after a Repo function is called)
  * `repo_opts`    - A keyword list of options given to the underlying repository operation

The following fields are private and must not be accessed directly.

  * `validations`
  * `constraints`
  * `filters`
  * `prepare`

### Redacting fields in inspect

To hide a field's value from the inspect protocol of `Ecto.Changeset`, mark
the field as `redact: true` in the schema, and it will display with the
value `**redacted**`.


---

## `change(data, changes \\ %{})`

Wraps the given data in a changeset or adds changes to a changeset.

`changes` is a map or keyword where the key is an atom representing a
field, association or embed and the value is a term. Note the `value` is
directly stored in the changeset with no validation whatsoever. For this
reason, this function is meant for working with data internal to the
application.

When changing embeds and associations, see `put_assoc/4` for a complete
reference on the accepted values.

This function is useful for:

  * wrapping a struct inside a changeset
  * directly changing a struct without performing castings nor validations
  * directly bulk-adding changes to a changeset

Changed attributes will only be added if the change does not have the
same value as the field in the data.

When a changeset is passed as the first argument, the changes passed as the
second argument are merged over the changes already in the changeset if they
differ from the values in the struct.

When a `{data, types}` is passed as the first argument, a changeset is
created with the given data and types and marked as valid.

See `cast/4` if you'd prefer to cast and validate external parameters.

## Examples

    iex> changeset = change(%Post{})
    %Ecto.Changeset{...}
    iex> changeset.valid?
    true
    iex> changeset.changes
    %{}

    iex> changeset = change(%Post{author: "bar"}, title: "title")
    iex> changeset.changes
    %{title: "title"}

    iex> changeset = change(%Post{title: "title"}, title: "title")
    iex> changeset.changes
    %{}

    iex> changeset = change(changeset, %{title: "new title", body: "body"})
    iex> changeset.changes.title
    "new title"
    iex> changeset.changes.body
    "body"


---

## `changed?(%Changeset{} = changeset, field, opts \\ [])`

Returns true if a field was changed in a changeset.

This function can check associations and embeds, but doesn't support the `:to`
and `:from` options for such fields.

## Options

  * `:to` - Check if the field was changed to a specific value
  * `:from` - Check if the field was changed from a specific value

## Examples

    iex> post = %Post{title: "Foo", body: "Old"}
    iex> changeset = change(post, %{title: "New title", body: "Old"})

    iex> changed?(changeset, :body)
    false

    iex> changed?(changeset, :title)
    true

    iex> changed?(changeset, :title, to: "NEW TITLE")
    false


---

## `empty_values`

The default list of empty values used by Ecto.

Defaults to `[""]`.


---

## `cast(data, params, permitted, opts \\ [])`

Applies the given `params` as changes on the `data` according to
the set of `permitted` keys. Returns a changeset.

`data` may be either a changeset, a schema struct or a `{data, types}`
tuple. The second argument is a map of `params` that are cast according
to the type information from `data`. `params` is a map with string keys
or a map with atom keys, containing potentially invalid data. Mixed keys
are not allowed.

During casting, all `permitted` parameters whose values match the specified
type information will have their key name converted to an atom and stored
together with the value as a change in the `:changes` field of the changeset.
If the cast value matches the current value for the field, it will not be
included in `:changes` unless the `force_changes: true` option is
provided. All parameters that are not explicitly permitted are ignored.

If casting of all fields is successful, the changeset is returned as valid.

Note that `cast/4` validates the types in the `params`, but not in the given
`data`.

## Options

  * `:trim_values` - a two arity function that trims values before checking if they
    belong to `changeset.empty_values`. Values that belong to empty values are replaced
    by the default value of the respective field (which is always nil for schemaless
    changesets). The default function is `&Ecto.Type.trim/2`

  * `:empty_values` - a list of values which are considered empty after trimming,
    defaults to `[""]`

  * `:force_changes` - a boolean indicating whether to include values that don't alter
    the current data in `:changes`. See `force_change/3` for more information, Defaults
    to `false`

  * `:message` - a function of arity 2 that is used to create the error message when
    casting fails. It is called for every field that cannot be casted and receives the
    field name as the first argument and the error metadata as the second argument. It
    must return a string or `nil`. If a string is returned it will be used as the error
    message. If `nil` is returned the default error message will be used. The field type
    is given under the `:type` key in the metadata

## Examples

    iex> changeset = cast(post, params, [:title])
    iex> if changeset.valid? do
    ...>   Repo.update!(changeset)
    ...> end

Passing a changeset as the first argument:

    iex> changeset = cast(post, %{title: "Hello"}, [:title])
    iex> new_changeset = cast(changeset, %{title: "Foo", body: "World"}, [:body])
    iex> new_changeset.params
    %{"title" => "Hello", "body" => "World"}

Or creating a changeset from a simple map with types:

    iex> data = %{title: "hello"}
    iex> types = %{title: :string}
    iex> changeset = cast({data, types}, %{title: "world"}, [:title])
    iex> apply_changes(changeset)
    %{title: "world"}

You can use empty values (and even cast multiple times) to change
what is considered an empty value:

    # Using default
    iex> params = %{title: "", topics: []}
    iex> changeset = cast(%Post{}, params, [:title, :topics])
    iex> changeset.changes
    %{topics: []}

    # Changing default
    iex> params = %{title: "", topics: []}
    iex> changeset = cast(%Post{}, params, [:title, :topics], empty_values: [[], nil])
    iex> changeset.changes
    %{title: ""}

    # Augmenting default
    iex> params = %{title: "", topics: []}
    iex> changeset =
    ...>   cast(%Post{}, params, [:title, :topics], empty_values: [[], nil] ++ Ecto.Changeset.empty_values())
    iex> changeset.changes
    %{}

You can define a custom error message function.

    # Using field name
    iex> params = %{title: 1, body: 2}
    iex> custom_errors = [title: "must be a string"]
    iex> msg_func = fn field, _meta -> custom_errors[field] end
    iex> changeset = cast(post, params, [:title, :body], message: msg_func)
    iex> changeset.errors
    [
      title: {"must be a string", [type: :string, validation: :cast]},
      body: {"is_invalid", [type: :string, validation: :cast]}
    ]

    # Using field type
    iex> params = %{title: 1, body: 2}
    iex> custom_errors = [string: "must be a string"]
    iex> msg_func = fn _field, meta ->
    ...    type = meta[:type]
    ...    custom_errors[type]
    ...  end
    iex> changeset = cast(post, params, [:title, :body], message: msg_func)
    iex> changeset.errors
    [
      title: {"must be a string", [type: :string, validation: :cast]},
      body: {"must be a string", [type: :string, validation: :cast]}
    ]

## Composing casts

`cast/4` also accepts a changeset as its first argument. In such cases, all
the effects caused by the call to `cast/4` (additional errors and changes)
are simply added to the ones already present in the argument changeset.
Parameters are merged (**not deep-merged**) and the ones passed to `cast/4`
take precedence over the ones already in the changeset.


---

## `cast_assoc(changeset, name, opts \\ [])`

Casts the given association with the changeset parameters.

This function should be used when working with the entire association at
once (and not a single element of a many-style association) and receiving
data external to the application.

`cast_assoc/3` matches the records extracted from the database
and compares it with the parameters received from an external source.
Therefore, it is expected that the data in the changeset has explicitly
preloaded the association being cast and that all of the IDs exist and
are unique.

For example, imagine a user has many addresses relationship where
post data is sent as follows

    %{"name" => "john doe", "addresses" => [
      %{"street" => "somewhere", "country" => "brazil", "id" => 1},
      %{"street" => "elsewhere", "country" => "poland"},
    ]}

and then

    User
    |> Repo.get!(id)
    |> Repo.preload(:addresses) # Only required when updating data
    |> Ecto.Changeset.cast(params, [])
    |> Ecto.Changeset.cast_assoc(:addresses, with: &MyApp.Address.changeset/2)

The parameters for the given association will be retrieved
from `changeset.params`. Those parameters are expected to be
a map with attributes, similar to the ones passed to `cast/4`.
Once parameters are retrieved, `cast_assoc/3` will match those
parameters with the associations already in the changeset data.

Once `cast_assoc/3` is called, Ecto will compare each parameter
with the user's already preloaded addresses and act as follows:

  * If the parameter does not contain an ID, the parameter data
    will be passed to `MyApp.Address.changeset/2` with a new struct
    and become an insert operation. We only consider the ID as not
    given if there is no "id" key or if its value is strictly `nil`

  * If the parameter contains an ID and there is no associated child
    with such ID, the parameter data will be passed to
    `MyApp.Address.changeset/2` with a new struct and become an insert
    operation

  * If the parameter contains an ID and there is an associated child
    with such ID, the parameter data will be passed to
    `MyApp.Address.changeset/2` with the existing struct and become an
    update operation

  * If there is an associated child with an ID and its ID is not given
    as parameter, the `:on_replace` callback for that association will
    be invoked (see the ["On replace" section](#module-the-on_replace-option)
    on the module documentation)

If two or more addresses have the same IDs, Ecto will consider that an
error and add an error to the changeset saying that there are duplicate
entries.

Every time the `MyApp.Address.changeset/2` function is invoked, it must
return a changeset. This changeset will always be included under `changes`
of the parent changeset, even if there are no changes. This is done for
reflection purposes, allowing developers to introspect validations and
other metadata from the association. Once the parent changeset is given
to an `Ecto.Repo` function, all entries will be inserted/updated/deleted
within the same transaction.

As you see above, this function is opinionated on how it works. If you
need different behaviour or if you need explicit control over the associated
data, you can either use `put_assoc/4` or use `Ecto.Multi` to encode how
several database operations will happen on several schemas and changesets
at once.

## Custom actions

Developers are allowed to explicitly set the `:action` field of a
changeset to instruct Ecto how to act in certain situations. Let's suppose
that, if one of the associations has only empty fields, you want to ignore
the entry altogether instead of showing an error. The changeset function could
be written like this:

    def changeset(struct, params) do
      struct
      |> cast(params, [:title, :body])
      |> validate_required([:title, :body])
      |> case do
        %{valid?: false, changes: changes} = changeset when changes == %{} ->
          # If the changeset is invalid and has no changes, it is
          # because all required fields are missing, so we ignore it.
          %{changeset | action: :ignore}
        changeset ->
          changeset
      end
    end

You can also set it to delete if you want data to be deleted based on the
received parameters (such as a checkbox or any other indicator).

## Partial changes for many-style associations

By preloading an association using a custom query you can confine the behavior
of `cast_assoc/3`. This opens up the possibility to work on a subset of the data,
instead of all associations in the database.

Taking the initial example of users having addresses, imagine those addresses
are set up to belong to a country. If you want to allow users to bulk edit all
addresses that belong to a single country, you can do so by changing the preload
query:

    query = from MyApp.Address, where: [country: ^edit_country]

    User
    |> Repo.get!(id)
    |> Repo.preload(addresses: query)
    |> Ecto.Changeset.cast(params, [])
    |> Ecto.Changeset.cast_assoc(:addresses)

This will allow you to cast and update only the association for the given country.
The important point for partial changes is that any addresses, which were not
preloaded won't be changed.

## Sorting and deleting from -many collections

In earlier examples, we passed a -many style association as a list:

    %{"name" => "john doe", "addresses" => [
      %{"street" => "somewhere", "country" => "brazil", "id" => 1},
      %{"street" => "elsewhere", "country" => "poland"},
    ]}

However, it is also common to pass the addresses as a map, where each
key is an integer representing its position:

    %{"name" => "john doe", "addresses" => %{
      0 => %{"street" => "somewhere", "country" => "brazil", "id" => 1},
      1 => %{"street" => "elsewhere", "country" => "poland"}
    }}

Using indexes becomes specially useful with two supporting options:
`:sort_param` and `:drop_param`. These options tell the indexes should
be reordered or deleted from the data. For example, if you did:

    cast_embed(changeset, :addresses,
      sort_param: :addresses_sort,
      drop_param: :addresses_drop)

You can now submit this:

    %{"name" => "john doe", "addresses" => %{...}, "addresses_drop" => [0]}

And now the entry with index 0 will be dropped from the params before casting.
Note this requires setting the relevant `:on_replace` option on your
associations/embeds definition.

Similar, for sorting, you could do:

    %{"name" => "john doe", "addresses" => %{...}, "addresses_sort" => [1, 0]}

And that will internally sort the elements so 1 comes before 0. Note that
any index not present in `"addresses_sort"` will come _before_ any of the
sorted indexes. If an index is not found, an empty entry is added in its
place.

For embeds, this guarantees the embeds will be rewritten in the given order.
However, for associations, this is not enough. You will have to add a
`field :position, :integer` to the schema and add a with function of arity 3
to add the position to your children changeset. For example, you could implement:

    defp child_changeset(child, _changes, position) do
      child
      |> change(position: position)
    end

And by passing it to `:with`, it will be called with the final position of the
item:

    changeset
    |> cast_assoc(:children, sort_param: ..., with: &child_changeset/3)

When using `:drop_param`, the remaining elements are re-indexed sequentially
starting from 0. For example, if you have the following parameters:

    %{
      "addresses" => %{
        0 => %{"street" => "First St"},
        1 => %{"street" => "Second St"},
        2 => %{"street" => "Third St"}
      },
      "addresses_drop" => [1]
    }

The element originally at index `2` ("Third St") will be passed to the `:with`
function with position `1`, because the list is compacted after the drop
operation.

These parameters can be powerful in certain UIs as it allows you to decouple
the sorting and replacement of the data from its representation.

## More resources

You can learn more about working with associations in our documentation,
including cheatsheets and practical examples. Check out:

  * The docs for `put_assoc/3`
  * The [associations cheatsheet](associations.html)
  * The [Constraints and Upserts guide](constraints-and-upserts.html)
  * The [Polymorphic associations with many to many guide](polymorphic-associations-with-many-to-many.html)

## Options

  * `:required` - if the association is a required field. For associations of cardinality
    one, a non-nil value satisfies this validation. For associations with many entries,
    a non-empty list is satisfactory.

  * `:required_message` - the message on failure, defaults to "can't be blank"

  * `:invalid_message` - the message on failure, defaults to "is invalid"

  * `:force_update_on_change` - force the parent record to be updated in the
    repository if there is a change, defaults to `true`

  * `:with` - the function to build the changeset from params. Defaults to the
    `changeset/2` function of the associated module. It can be an anonymous
    function that expects two arguments: the associated struct to be cast and its
    parameters. It must return a changeset. For associations with cardinality `:many`,
    functions with arity 3 are accepted, and the third argument will be the position
    of the associated element in the list, or `nil`, if the association is being replaced.

  * `:drop_param` - the parameter name which keeps a list of indexes to drop
    from the relation parameters

  * `:sort_param` - the parameter name which keeps a list of indexes to sort
    from the relation parameters. Unknown indexes are considered to be new
    entries. Non-listed indexes will come before any sorted ones. See
    `cast_assoc/3` for more information


---

## `reorder_assoc(%Changeset{} = changeset, name)`

Reorders the changes for a given association.

This function should be used when wanting to re-order the list of changes
for an association with cardinality `:many` before writing to the database.
The 2-arity version sorts the changes in a way that is safe for use with
unique constraints.

For example, if you have a unique constraint on the field `:name` and your list
of changes might introduce conflicts, you can use this to sort changes by deletes
first, then updates and then inserts. The `:on_replace` behaviour will be
handled automatically.

Using this function is preferable to relying on deferred constraints because the
resulting error cannot be mapped back into the correct changeset and your transaction
will simply raise.

Care must be taken when using this in conjunction with the `:sort_param` option
in `cast_assoc/3`. They both change the internal ordering of the association so you
must isolate the effects of this function to only the database operation.

See `reorder_assoc/3` if you would like to use a custom sorting function.

## Example
    iex> # assume `:comments` association has `on_replace: delete`
    iex> cs = %Post{comments: [%Comment{id: 1, body: "hello"}, %Comment{id: 2, body: "bye"}]}
    ...> |> change()
    ...> |> put_assoc(:comments, [%Comment{id: 3, body: ""}, %Comment{id: 2, body: "hello"}])
    ...> |> reorder_assoc(:comments, sort_fn)
    iex> cs.changes.comments
    [%Ecto.Changeset{data: %Comment{id: 1}}, %Ecto.Changeset{data: %Comment{id: 2}}, %Ecto.Changeset{data: %Comment{id: 3}}]


---

## `reorder_assoc(%Changeset{} = changeset, name, sort_fn)`

Reorders the changes for a given association using a custom sorting function.

Behaviour is similar to `reorder_assoc/2` except it allows the user to define
their own sorting function. It must be of arity 2 where the two arguments are
the changesets to be compared. You must return `true` if the first changeset
precedes or is in the same place as the second changeset and `false` otherwise.

## Example

    iex> sort_fn = cs1, _cs2 ->
    ...>   # ensure inserts come first
    ...>   case cs1.action do
    ...>     :insert -> true
    ...>      _ -> false
    ...>   end
    ...> end
    iex> cs = %Post{comments: [%Comment{id: 1, body: "hello"}]}
    ...> |> change()
    ...> |> put_assoc(:comments, [%Comment{id: 2, body: "hello"}, %Comment{id: 1, body: ""}])
    ...> |> reorder_assoc(:comments, sort_fn)
    iex> cs.changes.comments
    [%Ecto.Changeset{data: %Comment{id: 1}}, %Ecto.Changeset{data: %Comment{id: 2}}]


---

## `cast_embed(changeset, name, opts \\ [])`

Casts the given embed with the changeset parameters.

The parameters for the given embed will be retrieved
from `changeset.params`. Those parameters are expected to be
a map with attributes, similar to the ones passed to `cast/4`.
Once parameters are retrieved, `cast_embed/3` will match those
parameters with the embeds already in the changeset record.
See `cast_assoc/3` for an example of working with casts and
associations which would also apply for embeds.

The changeset must have been previously `cast` using
`cast/4` before this function is invoked.

## Options

  * `:required` - if the embed is a required field. For embeds of cardinality
    one, a non-nil value satisfies this validation. For embeds with many entries,
    a non-empty list is satisfactory.

  * `:required_message` - the message on failure, defaults to "can't be blank"

  * `:invalid_message` - the message on failure, defaults to "is invalid"

  * `:force_update_on_change` - force the parent record to be updated in the
    repository if there is a change, defaults to `true`

  * `:with` - the function to build the changeset from params. Defaults to the
    `changeset/2` function of the associated module. It must be an anonymous
    function that expects two arguments: the embedded struct to be cast and its
    parameters. It must return a changeset. For embeds with cardinality `:many`,
    functions with arity 3 are accepted, and the third argument will be the position
    of the associated element in the list, or `nil`, if the embed is being replaced.

  * `:drop_param` - the parameter name which keeps a list of indexes to drop
    from the relation parameters

  * `:sort_param` - the parameter name which keeps a list of indexes to sort
    from the relation parameters. Unknown indexes are considered to be new
    entries. Non-listed indexes will come before any sorted ones. See
    `cast_assoc/3` for more information


---

## `merge(changeset1, changeset2)`

Merges two changesets.

This function merges two changesets provided they have been applied to the
same data (their `:data` field is equal); if the data differs, an
`ArgumentError` exception is raised. If one of the changesets has a `:repo`
field which is not `nil`, then the value of that field is used as the `:repo`
field of the resulting changeset; if both changesets have a non-`nil` and
different `:repo` field, an `ArgumentError` exception is raised.

The other fields are merged with the following criteria:

  * `params` - params are merged (not deep-merged) giving precedence to the
    params of `changeset2` in case of a conflict. If both changesets have their
    `:params` fields set to `nil`, the resulting changeset will have its params
    set to `nil` too.
  * `changes` - changes are merged giving precedence to the `changeset2`
    changes.
  * `errors` and `validations` - they are simply concatenated.
  * `required` - required fields are merged; all the fields that appear
    in the required list of both changesets are moved to the required
    list of the resulting changeset.

## Examples

    iex> changeset1 = cast(%Post{}, %{title: "Title"}, [:title])
    iex> changeset2 = cast(%Post{}, %{title: "New title", body: "Body"}, [:title, :body])
    iex> changeset = merge(changeset1, changeset2)
    iex> changeset.changes
    %{body: "Body", title: "New title"}

    iex> changeset1 = cast(%Post{body: "Body"}, %{title: "Title"}, [:title])
    iex> changeset2 = cast(%Post{}, %{title: "New title"}, [:title])
    iex> merge(changeset1, changeset2)
    ** (ArgumentError) different :data when merging changesets


---

## `fetch_field(%Changeset{changes: changes, data: data, types: types}, key)`

Fetches the given field from changes or from the data.

While `fetch_change/2` only looks at the current `changes`
to retrieve a value, this function looks at the changes and
then falls back on the data, finally returning `:error` if
no value is available.

For relations, these functions will return the changeset
original data with changes applied. To retrieve raw changesets,
please use `fetch_change/2`.

## Examples

    iex> post = %Post{title: "Foo", body: "Bar baz bong"}
    iex> changeset = change(post, %{title: "New title"})
    iex> fetch_field(changeset, :title)
    {:changes, "New title"}
    iex> fetch_field(changeset, :body)
    {:data, "Bar baz bong"}
    iex> fetch_field(changeset, :not_a_field)
    :error


---

## `fetch_field!(changeset, key)`

Same as `fetch_field/2` but returns the value or raises if the given key was not found.

## Examples

    iex> post = %Post{title: "Foo", body: "Bar baz bong"}
    iex> changeset = change(post, %{title: "New title"})
    iex> fetch_field!(changeset, :title)
    "New title"
    iex> fetch_field!(changeset, :other)
    ** (KeyError) key :other not found in: %Post{...}


---

## `get_field(%Changeset{changes: changes, data: data, types: types}, key, default \\ nil)`

Gets a field from changes or from the data.

While `get_change/3` only looks at the current `changes`
to retrieve a value, this function looks at the changes and
then falls back on the data, finally returning `default` if
no value is available.

For associations and embeds, this function always returns
nil, a struct, or a list of structs. In case of changes,
the changeset data will have all data applies. This guarantees
a consistent result regardless if changes have been applied
or not. Use `get_change/2` or `get_assoc/3`/`get_embed/3`
if you want to retrieve the relations as changesets or
if you want more fine-grained control.

    iex> post = %Post{title: "A title", body: "My body is a cage"}
    iex> changeset = change(post, %{title: "A new title"})
    iex> get_field(changeset, :title)
    "A new title"
    iex> get_field(changeset, :not_a_field, "Told you, not a field!")
    "Told you, not a field!"


---

## `get_assoc(changeset, name, as \\ :changeset)`

Gets the association entry or entries from changes or from the data.

Returned data is normalized to changesets by default. Pass the `:struct`
flag to retrieve the data as structs with changes applied, similar to `get_field/2`.

## Examples

    iex> %Author{posts: [%Post{id: 1, title: "hello"}]}
    ...> |> change()
    ...> |> get_assoc(:posts)
    [%Ecto.Changeset{data: %Post{id: 1, title: "hello"}, changes: %{}}]

    iex> %Author{posts: [%Post{id: 1, title: "hello"}]}
    ...> |> cast(%{posts: [%{id: 1, title: "world"}]}, [])
    ...> |> cast_assoc(:posts)
    ...> |> get_assoc(:posts, :changeset)
    [%Ecto.Changeset{data: %Post{id: 1, title: "hello"}, changes: %{title: "world"}}]

    iex> %Author{posts: [%Post{id: 1, title: "hello"}]}
    ...> |> cast(%{posts: [%{id: 1, title: "world"}]}, [])
    ...> |> cast_assoc(:posts)
    ...> |> get_assoc(:posts, :struct)
    [%Post{id: 1, title: "world"}]


---

## `get_embed(changeset, name, as \\ :changeset)`

Gets the embedded entry or entries from changes or from the data.

Returned data is normalized to changesets by default. Pass the `:struct`
flag to retrieve the data as structs with changes applied, similar to `get_field/2`.

## Examples

    iex> %Post{comments: [%Comment{id: 1, body: "hello"}]}
    ...> |> change()
    ...> |> get_embed(:comments)
    [%Ecto.Changeset{data: %Comment{id: 1, body: "hello"}, changes: %{}}]

    iex> %Post{comments: [%Comment{id: 1, body: "hello"}]}
    ...> |> cast(%{comments: [%{id: 1, body: "world"}]}, [])
    ...> |> cast_embed(:comments)
    ...> |> get_embed(:comments, :changeset)
    [%Ecto.Changeset{data: %Comment{id: 1, body: "hello"}, changes: %{body: "world"}}]

    iex> %Post{comments: [%Comment{id: 1, body: "hello"}]}
    ...> |> cast(%{comments: [%{id: 1, body: "world"}]}, [])
    ...> |> cast_embed(:comments)
    ...> |> get_embed(:comments, :struct)
    [%Comment{id: 1, body: "world"}]


---

## `fetch_change(%Changeset{changes: changes} = _changeset, key)`

Fetches a change from the given changeset.

This function only looks at the `:changes` field of the given `changeset` and
returns `{:ok, value}` if the change is present or `:error` if it's not.

## Examples

    iex> changeset = change(%Post{body: "foo"}, %{title: "bar"})
    iex> fetch_change(changeset, :title)
    {:ok, "bar"}
    iex> fetch_change(changeset, :body)
    :error


---

## `fetch_change!(changeset, key)`

Same as `fetch_change/2` but returns the value or raises if the given key was not found.

## Examples

    iex> changeset = change(%Post{body: "foo"}, %{title: "bar"})
    iex> fetch_change!(changeset, :title)
    "bar"
    iex> fetch_change!(changeset, :body)
    ** (KeyError) key :body not found in: %{title: "bar"}


---

## `get_change(%Changeset{changes: changes} = _changeset, key, default \\ nil)`

Gets a change or returns a default value.

For associations and embeds, this function always returns
nil, a changeset, or a list of changesets.

## Examples

    iex> changeset = change(%Post{body: "foo"}, %{title: "bar"})
    iex> get_change(changeset, :title)
    "bar"
    iex> get_change(changeset, :body)
    nil


---

## `update_change(%Changeset{changes: changes} = changeset, key, function)`

Updates a change.

The given `function` is invoked with the change value only if there
is a change for `key`. Once the function is invoked, it behaves as
`put_change/3`.

Note that the value of the change can still be `nil` (unless the field
was marked as required on `validate_required/3`).

## Examples

    iex> changeset = change(%Post{}, %{impressions: 1})
    iex> changeset = update_change(changeset, :impressions, &(&1 + 1))
    iex> changeset.changes.impressions
    2


---

## `put_change(%Changeset{data: data, types: types} = changeset, key, value)`

Puts a change on the given `key` with `value`.

`key` is an atom that represents any field, embed or
association in the changeset. Note the `value` is directly
stored in the changeset with no validation whatsoever.
For this reason, this function is meant for working with
data internal to the application.

If the change is already present, it is overridden with
the new value. If the change has the same value as in the
changeset data, no changes are added (and any existing
changes are removed).

When changing embeds and associations, see `put_assoc/4`
for a complete reference on the accepted values.

## Examples

    iex> changeset = change(%Post{}, %{title: "foo"})
    iex> changeset = put_change(changeset, :title, "bar")
    iex> changeset.changes
    %{title: "bar"}

    iex> changeset = change(%Post{title: "foo"})
    iex> changeset = put_change(changeset, :title, "foo")
    iex> changeset.changes
    %{}


---

## `put_assoc(%Changeset{} = changeset, name, value, opts \\ [])`

Puts the given association entry or entries as a change in the changeset.

This function is used to work with associations as a whole. For example,
if a Post has many Comments, it allows you to add, remove or change all
comments at once, automatically computing inserts/updates/deletes by
comparing the data that you gave with the one already in the database.
If your goal is to manage individual resources, such as adding a new
comment to a post, or update post linked to a comment, then it is not
necessary to use this function. We will explore this later in the
["Example: Adding a comment to a post" section](#put_assoc/4-example-adding-a-comment-to-a-post).

This function requires the associated data to have been preloaded, except
when the parent changeset has been newly built and not yet persisted.
Missing data will invoke the `:on_replace` behaviour defined on the
association.

For associations with cardinality one, `nil` can be used to remove the existing
entry. For associations with many entries, an empty list may be given instead.

If the association has no changes, it will be skipped. If the association is
invalid, the changeset will be marked as invalid. If the given value is not any
of values below, it will raise.

The associated data may be given in different formats:

  * a map or a keyword list representing changes to be applied to the
    associated data. A map or keyword list can be given to update the
    associated data as long as they have matching primary keys.
    For example, `put_assoc(changeset, :comments, [%{id: 1, title: "changed"}])`
    will locate the comment with `:id` of 1 and update its title.
    If no comment with such id exists, one is created on the fly.
    Since only a single comment was given, any other associated comment
    will be replaced. On all cases, it is expected the keys to be atoms.
    Opposite to `cast_assoc` and `embed_assoc`, the given map (or struct)
    is not validated in any way and will be inserted as is.
    This API is mostly used in scripts and tests, to make it straight-
    forward to create schemas with associations at once, such as:

        Ecto.Changeset.change(
          %Post{},
          title: "foo",
          comments: [
            %{body: "first"},
            %{body: "second"}
          ]
        )

  * changesets - when changesets are given, they are treated as the canonical
    data and the associated data currently stored in the association is either
    updated or replaced. For example, if you call
    `put_assoc(post_changeset, :comments, [list_of_comments_changesets])`,
    all comments with matching IDs will be updated according to the changesets.
    New comments or comments not associated to any post will be correctly
    associated. Currently associated comments that do not have a matching ID
    in the list of changesets will act according to the `:on_replace` association
    configuration (you can choose to raise, ignore the operation, update or delete
    them). If there are changes in any of the changesets, they will be
    persisted too.

  * structs - when structs are given, they are treated as the canonical data
    and the associated data currently stored in the association is replaced.
    For example, if you call
    `put_assoc(post_changeset, :comments, [list_of_comments_structs])`,
    all comments with matching IDs will be replaced by the new structs.
    New comments or comments not associated to any post will be correctly
    associated. Currently associated comments that do not have a matching ID
    in the list of changesets will act according to the `:on_replace`
    association configuration (you can choose to raise, ignore the operation,
    update or delete them). Different to passing changesets, structs are not
    change tracked in any fashion. In other words, if you change a comment
    struct and give it to `put_assoc/4`, the updates in the struct won't be
    persisted. You must use changesets, keyword lists, or maps instead. `put_assoc/4` with structs
    only takes care of guaranteeing that the comments and the parent data
    are associated. This is extremely useful when associating existing data,
    as we will see in the ["Example: Adding tags to a post" section](#put_assoc/4-example-adding-tags-to-a-post).

Once the parent changeset is given to an `Ecto.Repo` function, all entries
will be inserted/updated/deleted within the same transaction.

If you need different behaviour or explicit control over how this function
behaves, you can drop it altogether and use `Ecto.Multi` to encode how several
database operations will happen on several schemas and changesets at once.

## Example: Adding a comment to a post

Imagine a relationship where Post has many comments and you want to add a
new comment to an existing post. While it is possible to use `put_assoc/4`
for this, it would be unnecessarily complex. Let's see an example.

First, let's fetch the post with all existing comments:

    post = Post |> Repo.get!(1) |> Repo.preload(:comments)

The following approach is **wrong**:

    post
    |> Ecto.Changeset.change()
    |> Ecto.Changeset.put_assoc(:comments, [%Comment{body: "bad example!"}])
    |> Repo.update!()

The reason why the example above is wrong is because `put_assoc/4` always
works with the **full data**. So the example above will effectively **erase
all previous comments** and only keep the comment you are currently adding.
Instead, you could try:

    post
    |> Ecto.Changeset.change()
    |> Ecto.Changeset.put_assoc(:comments, [%Comment{body: "so-so example!"} | post.comments])
    |> Repo.update!()

In this example, we prepend the new comment to the list of existing comments.
Ecto will diff the list of comments currently in `post` with the list of comments
given, and correctly insert the new comment to the database. Note, however,
Ecto is doing a lot of work just to figure out something we knew since the
beginning, which is that there is only one new comment.

In cases like above, when you want to work only on a single entry, it is
much easier to simply work on the association directly. For example, we
could instead set the `post` association in the comment:

    %Comment{body: "better example"}
    |> Ecto.Changeset.change()
    |> Ecto.Changeset.put_assoc(:post, post)
    |> Repo.insert!()

Alternatively, we can make sure that when we create a comment, it is already
associated to the post:

    Ecto.build_assoc(post, :comments)
    |> Ecto.Changeset.change(body: "great example!")
    |> Repo.insert!()

Or we can simply set the post_id in the comment itself:

    %Comment{body: "better example", post_id: post.id}
    |> Repo.insert!()

In other words, when you find yourself wanting to work only with a subset
of the data, then using `put_assoc/4` is most likely unnecessary. Instead,
you want to work on the other side of the association.

Let's see an example where using `put_assoc/4` is a good fit.

## Example: Adding tags to a post

Imagine you are receiving a set of tags you want to associate to a post.
Let's imagine that those tags exist upfront and are all persisted to the
database. Imagine we get the data in this format:

    params = %{"title" => "new post", "tags" => ["learner"]}

Now, since the tags already exist, we will bring all of them from the
database and put them directly in the post:

    tags = Repo.all(from t in Tag, where: t.name in ^params["tags"])

    post
    |> Repo.preload(:tags)
    |> Ecto.Changeset.cast(params, [:title]) # No need to allow :tags as we put them directly
    |> Ecto.Changeset.put_assoc(:tags, tags) # Explicitly set the tags

Since in this case we always require the user to pass all tags
directly, using `put_assoc/4` is a great fit. It will automatically
remove any tag not given and properly associate all of the given
tags with the post.

Furthermore, since the tag information is given as structs read directly
from the database, Ecto will treat the data as correct and only do the
minimum necessary to guarantee that posts and tags are associated,
without trying to update or diff any of the fields in the tag struct.

Although it accepts an `opts` argument, there are no options currently
supported by `put_assoc/4`.

## More resources

You can learn more about working with associations in our documentation,
including cheatsheets and practical examples. Check out:

  * The docs for `cast_assoc/3`
  * The [associations cheatsheet](associations.html)
  * The [Constraints and Upserts guide](constraints-and-upserts.html)
  * The [Polymorphic associations with many to many guide](polymorphic-associations-with-many-to-many.html)


---

## `put_embed(%Changeset{} = changeset, name, value, opts \\ [])`

Puts the given embed entry or entries as a change in the changeset.

This function is used to work with embeds as a whole. For embeds with
cardinality one, `nil` can be used to remove the existing entry. For
embeds with many entries, an empty list may be given instead.

If the embed has no changes, it will be skipped. If the embed is
invalid, the changeset will be marked as invalid.

The list of supported values and their behaviour is described in
`put_assoc/4`. If the given value is not any of values listed there,
it will raise.

Although this function accepts an `opts` argument, there are no options
currently supported by `put_embed/4`.


---

## `force_change(%Changeset{types: types} = changeset, key, value)`

Forces a change on the given `key` with `value`.

If the change is already present, it is overridden with
the new value. If the value is later modified via
`put_change/3` and `update_change/3`, reverting back to
its original value, the change will be reverted unless
`force_change/3` is called once again.

## Examples

    iex> changeset = change(%Post{author: "bar"}, %{title: "foo"})
    iex> changeset = force_change(changeset, :title, "bar")
    iex> changeset.changes
    %{title: "bar"}

    iex> changeset = force_change(changeset, :author, "bar")
    iex> changeset.changes
    %{title: "bar", author: "bar"}


---

## `delete_change(%Changeset{} = changeset, key)`

Deletes a change with the given key.

## Examples

    iex> changeset = change(%Post{}, %{title: "foo"})
    iex> changeset = delete_change(changeset, :title)
    iex> get_change(changeset, :title)
    nil


---

## `apply_changes(%Changeset{changes: changes, data: data})`

Applies the changeset changes to the changeset data.

This operation will return the underlying data with changes
regardless if the changeset is valid or not. See `apply_action/2`
for a similar function that ensures the changeset is valid.

## Examples

    iex> changeset = change(%Post{author: "bar"}, %{title: "foo"})
    iex> apply_changes(changeset)
    %Post{author: "bar", title: "foo"}


---

## `apply_action(%Changeset{} = changeset, action)`

Applies the changeset action only if the changes are valid.

If the changes are valid, all changes are applied to the changeset data.
If the changes are invalid, no changes are applied, and an error tuple
is returned with the changeset containing the action that was attempted
to be applied.

The action may be any atom.

## Examples

    iex> {:ok, data} = apply_action(changeset, :update)

    iex> {:ok, data} = apply_action(changeset, :my_action)

    iex> {:error, changeset} = apply_action(changeset, :update)
    %Ecto.Changeset{action: :update}


---

## `apply_action!(%Changeset{} = changeset, action)`

Applies the changeset action if the changes are valid or raises an error.

## Examples

    iex> changeset = change(%Post{author: "bar"}, %{title: "foo"})
    iex> apply_action!(changeset, :update)
    %Post{author: "bar", title: "foo"}

    iex> changeset = change(%Post{author: "bar"}, %{title: :bad})
    iex> apply_action!(changeset, :update)
    ** (Ecto.InvalidChangesetError) could not perform update because changeset is invalid.

See `apply_action/2` for more information.


---

## `validations(%Changeset{validations: validations})`

Returns a keyword list of the validations for this changeset.

The keys in the list are the names of fields, and the values are a
validation associated with the field. A field may occur multiple
times in the list.

## Example

    %Post{}
    |> change()
    |> validate_format(:title, ~r/^\w+:\s/, message: "must start with a topic")
    |> validate_length(:title, max: 100)
    |> validations()
    #=> [
      title: {:length, [ max: 100 ]},
      title: {:format, ~r/^\w+:\s/}
    ]

The following validations may be included in the result. The list is
not necessarily exhaustive. For example, custom validations written
by the developer will also appear in our return value.

This first group contains validations that hold a keyword list of validators.
This list may also include a `:message` key.

  * `{:length, [option]}`

    * `min: n`
    * `max: n`
    * `is: n`
    * `count: :graphemes | :codepoints`

  * `{:number,  [option]}`

    * `equal_to: n`
    * `greater_than: n`
    * `greater_than_or_equal_to: n`
    * `less_than: n`
    * `less_than_or_equal_to: n`

The other validators simply take a value:

  * `{:exclusion, Enum.t}`
  * `{:format, ~r/pattern/}`
  * `{:inclusion, Enum.t}`
  * `{:subset, Enum.t}`

Note that calling `validate_required/3` does not store the validation under the
`changeset.validations` key (and so won't be included in the result of this
function). The required fields are stored under the `changeset.required` key.


---

## `add_error(%Changeset{errors: errors} = changeset, key, message, keys \\ [])`

Adds an error to the changeset.

An additional keyword list `keys` can be passed to provide additional
contextual information for the error. This is useful when using
`traverse_errors/2` and when translating errors with `Gettext`

## Examples

    iex> changeset = change(%Post{}, %{title: ""})
    iex> changeset = add_error(changeset, :title, "empty")
    iex> changeset.errors
    [title: {"empty", []}]
    iex> changeset.valid?
    false

    iex> changeset = change(%Post{}, %{title: ""})
    iex> changeset = add_error(changeset, :title, "empty", additional: "info")
    iex> changeset.errors
    [title: {"empty", [additional: "info"]}]
    iex> changeset.valid?
    false

    iex> changeset = change(%Post{}, %{tags: ["ecto", "elixir", "x"]})
    iex> changeset = add_error(changeset, :tags, "tag '%{val}' is too short", val: "x")
    iex> changeset.errors
    [tags: {"tag '%{val}' is too short", [val: "x"]}]
    iex> changeset.valid?
    false


---

## `validate_change(%Changeset{} = changeset, field, validator)`

Validates the given `field` change.

It invokes the `validator` function to perform the validation
only if a change for the given `field` exists and the change
value is not `nil`. The function must return a list of errors
(with an empty list meaning no errors).

In case there's at least one error, the list of errors will be appended to the
`:errors` field of the changeset and the `:valid?` flag will be set to
`false`.

## Examples

    iex> changeset = change(%Post{}, %{title: "foo"})
    iex> changeset = validate_change changeset, :title, fn :title, title  ->
    ...>   # Value must not be "foo"!
    ...>   if title == "foo" do
    ...>     [title: "cannot be foo"]
    ...>   else
    ...>     []
    ...>   end
    ...> end
    iex> changeset.errors
    [title: {"cannot be foo", []}]

    iex> changeset = change(%Post{}, %{title: "foo"})
    iex> changeset = validate_change changeset, :title, fn :title, title  ->
    ...>   if title == "foo" do
    ...>     [title: {"cannot be foo", additional: "info"}]
    ...>   else
    ...>     []
    ...>   end
    ...> end
    iex> changeset.errors
    [title: {"cannot be foo", [additional: "info"]}]


---

## `validate_change( %Changeset{validations: validations} = changeset, field, metadata, validator )`

Stores the validation `metadata` and validates the given `field` change.

Similar to `validate_change/3` but stores the validation metadata
into the changeset validators. The validator metadata is often used
as a reflection mechanism, to automatically generate code based on
the available validations.

## Examples

    iex> changeset = change(%Post{}, %{title: "foo"})
    iex> changeset = validate_change changeset, :title, :useless_validator, fn
    ...>   _, _ -> []
    ...> end
    iex> changeset.validations
    [title: :useless_validator]


---

## `validate_required(%Changeset{} = changeset, fields, opts \\ [])`

Validates that one or more fields are present in the changeset.

You can pass a single field name or a list of field names that
are required.

If the value of a field is `nil` or matches any of the values
configured in `changeset.empty_values` (by default `[]` and `""`),
the changeset is marked as invalid, the field is removed from the
changeset's changes, and an error is added. An error won't be added
if the field already has an error.

Do not use this function to validate associations that are required,
instead pass the `:required` option to `cast_assoc/3` or `cast_embed/3`.

Opposite to other validations, calling this function does not store
the validation under the `changeset.validations` key. Instead, it
stores all required fields under `changeset.required`.

## Options

  * `:message` - the message on failure, defaults to "can't be blank".
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.

## Examples

    validate_required(changeset, :title)
    validate_required(changeset, [:title, :body])


---

## `field_missing?(%Changeset{types: types} = changeset, field)`

Determines whether a field is missing in a changeset.

A field is considered missing if it is `nil` or it matches any
of the values configured in `changeset.empty_values`.

This is useful when performing complex validations that are not possible with
`validate_required/3`. For example, evaluating whether at least one field
from a list is present or evaluating that exactly one field from a list is
present.

## Examples

    iex> changeset = cast(%Post{}, %{color: "Red"}, [:color])
    iex> missing_fields = Enum.filter([:title, :body], &field_missing?(changeset, &1))
    iex> changeset =
    ...>   case missing_fields do
    ...>     [_, _] -> add_error(changeset, :title, "at least one of `:title` or `:body` must be present")
    ...>     _ -> changeset
    ...>   end
    ...> changeset.errors
    [title: {"at least one of `:title` or `:body` must be present", []}]


---

## `unsafe_validate_unique(%Changeset{} = changeset, fields, repo, opts \\ [])`

Validates that no existing record with a different primary key
has the same values for these fields.

This function exists to provide quick feedback to users of your
application. It should not be relied on for any data guarantee as it
has race conditions and is inherently unsafe. For example, if this
check happens twice in the same time interval (because the user
submitted a form twice), both checks may pass and you may end-up with
duplicate entries in the database. Therefore, a `unique_constraint/3`
should also be used to ensure your data won't get corrupted.

However, because constraints are only checked if all validations
succeed, this function can be used as an early check to provide
early feedback to users, since most conflicting data will have been
inserted prior to the current validation phase.

When applying this validation to a schemas loaded from the database
this check will exclude rows having the same primary key as set on
the changeset, as those are supposed to be overwritten anyways.

## Options

  * `:message` - the message in case the constraint check fails,
    defaults to "has already been taken". Can also be a `{msg, opts}` tuple,
    to provide additional options when using `traverse_errors/2`.

  * `:error_key` - the key to which changeset error will be added when
    check fails, defaults to the first field name of the given list of
    fields.

  * `:prefix` - the prefix to run the query on (such as the schema path
    in Postgres or the database in MySQL). See `Ecto.Repo` documentation
    for more information.

  * `:nulls_distinct` - a boolean controlling whether different null values
    are considered distinct (not equal). If `false`, `nil` values will have
    their uniqueness checked. Otherwise, the check will not be performed. This
    is only meaningful when paired with a unique index that treats nulls as equal,
    such as Postgres 15's `NULLS NOT DISTINCT` option. Defaults to `true`

  * `:repo_opts` - the options to pass to the `Ecto.Repo` call.

  * `:query` - the base query to use for the check. Defaults to the schema of
    the changeset. If the primary key is set, a clause will be added to exclude
    the changeset row itself from the check.

## Examples

    unsafe_validate_unique(changeset, :city_name, repo)
    unsafe_validate_unique(changeset, [:city_name, :state_name], repo)
    unsafe_validate_unique(changeset, [:city_name, :state_name], repo, message: "city must be unique within state")
    unsafe_validate_unique(changeset, [:city_name, :state_name], repo, prefix: "public")
    unsafe_validate_unique(changeset, [:city_name, :state_name], repo, query: from(c in City, where: is_nil(c.deleted_at)))


---

## `validate_format(changeset, field, format, opts \\ [])`

Validates a change has the given format.

The format has to be expressed as a regular expression.

The validation only runs if a change for the given `field` exists and the
change value is not `nil`.

## Options

  * `:message` - the message on failure, defaults to "has invalid format".
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.

## Examples

    validate_format(changeset, :email, ~r/@/)


---

## `validate_inclusion(changeset, field, data, opts \\ [])`

Validates a change is included in the given enumerable.

The validation only runs if a change for the given `field` exists and the
change value is not `nil`.

## Options

  * `:message` - the message on failure, defaults to "is invalid".
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.

## Examples

    validate_inclusion(changeset, :cardinal_direction, ["north", "east", "south", "west"])
    validate_inclusion(changeset, :age, 0..99)


---

## `validate_subset(changeset, field, data, opts \\ [])`

Validates a change, of type enum, is a subset of the given enumerable.

This validates if a list of values belongs to the given enumerable.
If you need to validate if a single value is inside the given enumerable,
you should use `validate_inclusion/4` instead.

Type of the field must be array.

The validation only runs if a change for the given `field` exists and the
change value is not `nil`.

## Options

  * `:message` - the message on failure, defaults to "has an invalid entry".
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.

## Examples

    validate_subset(changeset, :pets, ["cat", "dog", "parrot"])
    validate_subset(changeset, :lottery_numbers, 0..99)


---

## `validate_exclusion(changeset, field, data, opts \\ [])`

Validates a change is not included in the given enumerable.

The validation only runs if a change for the given `field` exists and the
change value is not `nil`.

## Options

  * `:message` - the message on failure, defaults to "is reserved".
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.

## Examples

    validate_exclusion(changeset, :name, ~w(admin superadmin))


---

## `validate_length(changeset, field, opts)`

Validates a change is a string or list of the given length.

Note that the length of a string is counted in graphemes by default. If using
this validation to match a character limit of a database backend,
it's likely that the limit ignores graphemes and limits the number
of unicode characters. Then consider using the `:count` option to
limit the number of codepoints (`:codepoints`), or limit the number of bytes (`:bytes`).

The validation only runs if a change for the given `field` exists and the
change value is not `nil`.

## Options

  * `:is` - the length must be exactly this value
  * `:min` - the length must be greater than or equal to this value
  * `:max` - the length must be less than or equal to this value
  * `:count` - what length to count for string, `:graphemes` (default), `:codepoints` or `:bytes`
  * `:message` - the message on failure, depending on the validation, is one of:
    * for strings:
      * "should be %{count} character(s)"
      * "should be at least %{count} character(s)"
      * "should be at most %{count} character(s)"
    * for binary:
      * "should be %{count} byte(s)"
      * "should be at least %{count} byte(s)"
      * "should be at most %{count} byte(s)"
    * for lists and maps:
      * "should have %{count} item(s)"
      * "should have at least %{count} item(s)"
      * "should have at most %{count} item(s)"
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.

## Examples

    validate_length(changeset, :title, min: 3)
    validate_length(changeset, :title, max: 100)
    validate_length(changeset, :title, min: 3, max: 100)
    validate_length(changeset, :code, is: 9)
    validate_length(changeset, :topics, is: 2)
    validate_length(changeset, :icon, count: :bytes, max: 1024 * 16)


---

## `validate_number(changeset, field, opts)`

Validates the properties of a number.

The validation only runs if a change for the given `field` exists and the
change value is not `nil`.

## Options

  * `:less_than`
  * `:greater_than`
  * `:less_than_or_equal_to`
  * `:greater_than_or_equal_to`
  * `:equal_to`
  * `:not_equal_to`
  * `:message` - the message on failure, defaults to one of:
    * "must be less than %{number}"
    * "must be greater than %{number}"
    * "must be less than or equal to %{number}"
    * "must be greater than or equal to %{number}"
    * "must be equal to %{number}"
    * "must be not equal to %{number}"
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.

## Examples

    validate_number(changeset, :count, less_than: 3)
    validate_number(changeset, :pi, greater_than: 3, less_than: 4)
    validate_number(changeset, :the_answer_to_life_the_universe_and_everything, equal_to: 42)


---

## `validate_confirmation(changeset, field, opts \\ [])`

Validates that the given parameter matches its confirmation.

By calling `validate_confirmation(changeset, :email)`, this
validation will check if both "email" and "email_confirmation"
in the parameter map matches. Note this validation only looks
at the parameters themselves, never the fields in the schema.
As such as, the "email_confirmation" field does not need to be
added as a virtual field in your schema.

Note that if the confirmation field is missing, this does not
add a validation error. This is done on purpose as you do not
trigger confirmation validation in places where a confirmation
is not required (for example, in APIs). You can force the
confirmation parameter to be required in the options (see below).

## Options

  * `:message` - the message on failure, defaults to "does not match confirmation".
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.
  * `:required` - boolean, sets whether existence of confirmation parameter
    is required for addition of error. Defaults to false

## Examples

    validate_confirmation(changeset, :email)
    validate_confirmation(changeset, :password, message: "does not match password")

    cast(data, params, [:password])
    |> validate_confirmation(:password, message: "does not match password")


---

## `validate_acceptance(changeset, field, opts \\ [])`

Validates the given parameter is true.

Note this validation only checks the parameter itself is true, never
the field in the schema. That's because acceptance parameters do not need
to be persisted, as by definition they would always be stored as `true`.

## Options

  * `:message` - the message on failure, defaults to "must be accepted".
    Can also be a `{msg, opts}` tuple, to provide additional options
    when using `traverse_errors/2`.

## Examples

    validate_acceptance(changeset, :terms_of_service)
    validate_acceptance(changeset, :rules, message: "please accept rules")


---

## `optimistic_lock(data_or_changeset, field, incrementer \\ &increment_with_rollover/1)`

Applies optimistic locking to the changeset.

[Optimistic
locking](https://en.wikipedia.org/wiki/Optimistic_concurrency_control) (or
*optimistic concurrency control*) is a technique that allows concurrent edits
on a single record. While pessimistic locking works by locking a resource for
an entire transaction, optimistic locking only checks if the resource changed
before updating it.

This is done by regularly fetching the record from the database, then checking
whether another user has made changes to the record *only when updating the
record*. This behaviour is ideal in situations where the chances of concurrent
updates to the same record are low; if they're not, pessimistic locking or
other concurrency patterns may be more suited.

## Usage

Optimistic locking works by keeping a "version" counter for each record; this
counter gets incremented each time a modification is made to a record. Hence,
in order to use optimistic locking, a field must exist in your schema for
versioning purpose. Such field is usually an integer but other types are
supported.

## Examples

Assuming we have a `Post` schema (stored in the `posts` table), the first step
is to add a version column to the `posts` table:

    alter table(:posts) do
      add :lock_version, :integer, default: 1
    end

The column name is arbitrary and doesn't need to be `:lock_version`. Now add
a field to the schema too:

    defmodule Post do
      use Ecto.Schema

      schema "posts" do
        field :title, :string
        field :lock_version, :integer, default: 1
      end

      def changeset(:update, struct, params \\ %{}) do
        struct
        |> Ecto.Changeset.cast(params, [:title])
        |> Ecto.Changeset.optimistic_lock(:lock_version)
      end
    end

Now let's take optimistic locking for a spin:

    iex> post = Repo.insert!(%Post{title: "foo"})
    %Post{id: 1, title: "foo", lock_version: 1}
    iex> valid_change = Post.changeset(:update, post, %{title: "bar"})
    iex> stale_change = Post.changeset(:update, post, %{title: "baz"})
    iex> Repo.update!(valid_change)
    %Post{id: 1, title: "bar", lock_version: 2}
    iex> Repo.update!(stale_change)
    ** (Ecto.StaleEntryError) attempted to update a stale entry:

    %Post{id: 1, title: "baz", lock_version: 1}

When a conflict happens (a record which has been previously fetched is
being updated, but that same record has been modified since it was
fetched), an `Ecto.StaleEntryError` exception is raised.

Optimistic locking also works with delete operations. Just call the
`optimistic_lock/3` function with the data before delete:

    iex> changeset = Ecto.Changeset.optimistic_lock(post, :lock_version)
    iex> Repo.delete(changeset)

`optimistic_lock/3` by default assumes the field
being used as a lock is an integer. If you want to use another type,
you need to pass the third argument customizing how the next value
is generated:

    iex> Ecto.Changeset.optimistic_lock(post, :lock_uuid, fn _ -> Ecto.UUID.generate end)


---

## `prepare_changes(%Changeset{prepare: prepare} = changeset, function)`

Provides a function executed by the repository on insert/update/delete.

If the changeset given to the repository is valid, the function given to
`prepare_changes/2` will be called with the changeset and must return a
changeset, allowing developers to do final adjustments to the changeset or
to issue data consistency commands. The repository itself can be accessed
inside the function under the `repo` field in the changeset. If the
changeset given to the repository is invalid, the function will not be
invoked.

The given function is guaranteed to run inside the same transaction
as the changeset operation for databases that do support transactions.

## Example

A common use case is updating a counter cache, in this case updating a post's
comment count when a comment is created:

    def create_comment(comment, params) do
      comment
      |> cast(params, [:body, :post_id])
      |> prepare_changes(fn changeset ->
           if post_id = get_change(changeset, :post_id) do
             query = from Post, where: [id: ^post_id]
             changeset.repo.update_all(query, inc: [comment_count: 1])
           end
           changeset
         end)
    end

We retrieve the repo from the comment changeset itself and use
update_all to update the counter cache in one query. Finally, the original
changeset must be returned.


---

## `constraints(%Changeset{constraints: constraints})`

Returns all constraints in a changeset.

A constraint is a map with the following fields:

  * `:type` - the type of the constraint that will be checked in the database,
    such as `:check`, `:unique`, etc
  * `:constraint` - the database constraint name as a string or `Regex`. The constraint at
    the database level will be checked against this according to `:match` type
  * `:match` - the type of match Ecto will perform on a violated constraint
    against the `:constraint` value. It is `:exact`, `:suffix` or `:prefix`
  * `:field` - the field a violated constraint will apply the error to
  * `:error_message` - the error message in case of violated constraints
  * `:error_type` - the type of error that identifies the error message


---

## `check_constraint(changeset, field, opts \\ [])`

Checks for a check constraint in the given field.

The check constraint works by relying on the database to check
if the check constraint has been violated or not and, if so,
Ecto converts it into a changeset error.

In order to use the check constraint, the first step is
to define the check constraint in a migration:

    create constraint("users", :age_must_be_positive, check: "age > 0")

Now that a constraint exists, when modifying users, we could
annotate the changeset with a check constraint so Ecto knows
how to convert it into an error message:

    cast(user, params, [:age])
    |> check_constraint(:age, name: :age_must_be_positive)

Now, when invoking `c:Ecto.Repo.insert/2` or `c:Ecto.Repo.update/2`,
if the age is not positive, the underlying operation will fail
but Ecto will convert the database exception into a changeset error
and return an `{:error, changeset}` tuple. Note that the error will
occur only after hitting the database, so it will not be visible
until all other validations pass. If the constraint fails inside a
transaction, the transaction will be marked as aborted.

## Options

  * `:message` - the message in case the constraint check fails.
    Defaults to "is invalid"
  * `:name` - the constraint name. By default, the constraint
    name is inferred from the table + field. If this option is given,
    the `field` argument only indicates the field the error will be
    added to. May be required explicitly for complex cases
  * `:match` - how the changeset constraint name is matched against the
    repo constraint, may be `:exact`, `:suffix` or `:prefix`. Defaults to
    `:exact`. `:suffix` matches any repo constraint which `ends_with?` `:name`
    to this changeset constraint. `:prefix` matches any repo constraint which
    `starts_with?` `:name` to this changeset constraint.


---

## `unique_constraint(changeset, field_or_fields, opts \\ [])`

Checks for a unique constraint in the given field or list of fields.

The unique constraint works by relying on the database to check
if the unique constraint has been violated or not and, if so,
Ecto converts it into a changeset error.

In order to use the uniqueness constraint, the first step is
to define the unique index in a migration:

    create unique_index(:users, [:email])

Now that a constraint exists, when modifying users, we could
annotate the changeset with a unique constraint so Ecto knows
how to convert it into an error message:

    cast(user, params, [:email])
    |> unique_constraint(:email)

Now, when invoking `c:Ecto.Repo.insert/2` or `c:Ecto.Repo.update/2`,
if the email already exists, the underlying operation will fail but
Ecto will convert the database exception into a changeset error and
return an `{:error, changeset}` tuple. Note that the error will occur
only after hitting the database, so it will not be visible until all
other validations pass. If the constraint fails inside a transaction,
the transaction will be marked as aborted.

## Options

  * `:message` - the message in case the constraint check fails,
    defaults to "has already been taken"

  * `:name` - the constraint name. By default, the constraint
    name is inferred from the table + field. If this option is given,
    the `field` argument only indicates the field the error will be
    added to. May be required explicitly for complex cases

  * `:match` - how the changeset constraint name is matched against the
    repo constraint, may be `:exact`, `:suffix` or `:prefix`. Defaults to
    `:exact`. `:suffix` matches any repo constraint which `ends_with?` `:name`
    to this changeset constraint. `:prefix` matches any repo constraint which
    `starts_with?` `:name` to this changeset constraint.

  * `:error_key` - the key to which changeset error will be added when
    check fails, defaults to the first field name of the given list of
    fields.

## Complex constraints

Because the constraint logic is in the database, we can leverage
all the database functionality when defining them. For example,
let's suppose the e-mails are scoped by company id:

    # In migration
    create unique_index(:users, [:email, :company_id])

    # In the changeset function
    cast(user, params, [:email])
    |> unique_constraint([:email, :company_id])

The first field name, `:email` in this case, will be used as the error
key to the changeset errors keyword list. For example, the above
`unique_constraint/3` would generate something like:

    Repo.insert!(%User{email: "john@elixir.org", company_id: 1})
    changeset = User.changeset(%User{}, %{email: "john@elixir.org", company_id: 1})
    {:error, changeset} = Repo.insert(changeset)
    changeset.errors #=> [email: {"has already been taken", []}]

In complex cases, instead of relying on name inference, it may be best
to set the constraint name explicitly:

    # In the migration
    create unique_index(:users, [:email, :company_id], name: :users_email_company_id_index)

    # In the changeset function
    cast(user, params, [:email])
    |> unique_constraint(:email, name: :users_email_company_id_index)

### Partitioning

If your table is partitioned, then your unique index might look different
per partition, e.g. Postgres adds p<number> to the middle of your key, like:

    users_p0_email_key
    users_p1_email_key
    ...
    users_p99_email_key

In this case you can use the name and suffix options together to match on
these dynamic indexes, like:

    cast(user, params, [:email])
    |> unique_constraint(:email, name: :email_key, match: :suffix)

There are cases where the index has a number added both for table name and
index name, generating an index name such as:

    user_p0_email_idx2
    user_p1_email_idx3
    ...
    user_p99_email_idx101

In that case, a `Regex` can be used to match:

    cast(user, params, [:email])
    |> unique_constraint(:email, name: ~r/user_p\d+_email_idx\d+/)

## Case sensitivity

Unfortunately, different databases provide different guarantees
when it comes to case-sensitiveness. For example, in MySQL, comparisons
are case-insensitive by default. In Postgres, users can define case
insensitive column by using the `:citext` type/extension. In your migration:

    execute "CREATE EXTENSION IF NOT EXISTS citext"
    create table(:users) do
      ...
      add :email, :citext
      ...
    end

If for some reason your database does not support case insensitive columns,
you can explicitly downcase values before inserting/updating them:

    cast(data, params, [:email])
    |> update_change(:email, &String.downcase/1)
    |> unique_constraint(:email)


---

## `foreign_key_constraint(changeset, field, opts \\ [])`

Checks for foreign key constraint in the given field.

The foreign key constraint works by relying on the database to
check if the associated data exists or not. This is useful to
guarantee that a child will only be created if the parent exists
in the database too.

In order to use the foreign key constraint the first step is
to define the foreign key in a migration. This is often done
with references. For example, imagine you are creating a
comments table that belongs to posts. One would have:

    create table(:comments) do
      add :post_id, references(:posts)
    end

By default, Ecto will generate a foreign key constraint with
name "comments_post_id_fkey" (the name is configurable).

Now that a constraint exists, when creating comments, we could
annotate the changeset with foreign key constraint so Ecto knows
how to convert it into an error message:

    cast(comment, params, [:post_id])
    |> foreign_key_constraint(:post_id)

Now, when invoking `c:Ecto.Repo.insert/2` or `c:Ecto.Repo.update/2`,
if the associated post does not exist, the underlying operation will
fail but Ecto will convert the database exception into a changeset
error and return an `{:error, changeset}` tuple. Note that the error
will occur only after hitting the database, so it will not be visible
until all other validations pass. If the constraint fails inside a
transaction, the transaction will be marked as aborted.

## Options

  * `:message` - the message in case the constraint check fails,
    defaults to "does not exist"
  * `:name` - the constraint name. By default, the constraint
    name is inferred from the table + field. If this option is given,
    the `field` argument only indicates the field the error will be
    added to. May be required explicitly for complex cases
  * `:match` - how the changeset constraint name is matched against the
    repo constraint, may be `:exact`, `:suffix` or `:prefix`. Defaults to
    `:exact`. `:suffix` matches any repo constraint which `ends_with?` `:name`
    to this changeset constraint. `:prefix` matches any repo constraint which
    `starts_with?` `:name` to this changeset constraint.


---

## `assoc_constraint(changeset, assoc, opts \\ [])`

Checks the associated field exists.

This is similar to `foreign_key_constraint/3` except that the
field is inferred from the association definition. This is useful
to guarantee that a child will only be created if the parent exists
in the database too. Therefore, it only applies to `belongs_to`
associations.

As the name says, a constraint is required in the database for
this function to work. Such constraint is often added as a
reference to the child table:

    create table(:comments) do
      add :post_id, references(:posts)
    end

Now, when inserting a comment, it is possible to forbid any
comment to be added if the associated post does not exist:

    comment
    |> Ecto.Changeset.cast(params, [:post_id])
    |> Ecto.Changeset.assoc_constraint(:post)
    |> Repo.insert

## Options

  * `:message` - the message in case the constraint check fails,
    defaults to "does not exist"
  * `:name` - the constraint name. By default, the constraint
    name is inferred from the table + field. If this option is given,
    the `field` argument only indicates the field the error will be
    added to. May be required explicitly for complex cases
  * `:match` - how the changeset constraint name is matched against the
    repo constraint, may be `:exact`, `:suffix` or `:prefix`. Defaults to
    `:exact`. `:suffix` matches any repo constraint which `ends_with?` `:name`
    to this changeset constraint. `:prefix` matches any repo constraint which
    `starts_with?` `:name` to this changeset constraint.


---

## `no_assoc_constraint(changeset, assoc, opts \\ [])`

Checks the associated field does not exist.

This is similar to `foreign_key_constraint/3` except that the
field is inferred from the association definition. This is useful
to guarantee that parent can only be deleted (or have its primary
key changed) if no child exists in the database. Therefore, it only
applies to `has_*` associations.

As the name says, a constraint is required in the database for
this function to work. Such constraint is often added as a
reference to the child table:

    create table(:comments) do
      add :post_id, references(:posts)
    end

Now, when deleting the post, it is possible to forbid any post to
be deleted if they still have comments attached to it:

    post
    |> Ecto.Changeset.change
    |> Ecto.Changeset.no_assoc_constraint(:comments)
    |> Repo.delete

## Options

  * `:message` - the message in case the constraint check fails,
    defaults to "is still associated with this entry" (for `has_one`)
    and "are still associated with this entry" (for `has_many`)
  * `:name` - the constraint name. By default, the constraint
    name is inferred from the table + field. If this option is given,
    the `field` argument only indicates the field the error will be
    added to. May be required explicitly for complex cases
  * `:match` - how the changeset constraint name is matched against the
    repo constraint, may be `:exact`, `:suffix` or `:prefix`. Defaults to
    `:exact`. `:suffix` matches any repo constraint which `ends_with?` `:name`
    to this changeset constraint. `:prefix` matches any repo constraint which
    `starts_with?` `:name` to this changeset constraint.


---

## `exclusion_constraint(changeset, field, opts \\ [])`

Checks for an exclusion constraint in the given field.

The exclusion constraint works by relying on the database to check
if the exclusion constraint has been violated or not and, if so,
Ecto converts it into a changeset error.

## Options

  * `:message` - the message in case the constraint check fails,
    defaults to "violates an exclusion constraint"
  * `:name` - the constraint name. By default, the constraint
    name is inferred from the table + field. If this option is given,
    the `field` argument only indicates the field the error will be
    added to. May be required explicitly for complex cases
  * `:match` - how the changeset constraint name is matched against the
    repo constraint, may be `:exact`, `:suffix` or `:prefix`. Defaults to
    `:exact`. `:suffix` matches any repo constraint which `ends_with?` `:name`
    to this changeset constraint. `:prefix` matches any repo constraint which
    `starts_with?` `:name` to this changeset constraint.


---

## `traverse_errors( %Changeset{errors: errors, changes: changes, types: types} = changeset, msg_func )`

Traverses changeset errors and applies the given function to error messages.

This function is particularly useful when associations and embeds
are cast in the changeset as it will traverse all associations and
embeds and place all errors in a series of nested maps.

A changeset is supplied along with a function to apply to each
error message as the changeset is traversed. The error message
function receives an error tuple `{msg, opts}`, for example:

    {"should be at least %{count} characters", [count: 3, validation: :length, min: 3]}

## Examples

    iex> traverse_errors(changeset, fn {msg, opts} ->
    ...>   Regex.replace(~r"%{(\w+)}", msg, fn _, key ->
    ...>     opts |> Keyword.get(String.to_existing_atom(key), key) |> to_string()
    ...>   end)
    ...> end)
    %{title: ["should be at least 3 characters"]}

Optionally function can accept three arguments: `changeset`, `field`
and error tuple `{msg, opts}`. It is useful whenever you want to extract
validations rules from `changeset.validations` to build detailed error
description.


---

## `traverse_validations( %Changeset{validations: validations, changes: changes, types: types} = changeset, msg_fu …`

Traverses changeset validations and applies the given function to validations.

This behaves the same as `traverse_errors/2`, but operates on changeset
validations instead of errors.

## Examples

    iex> traverse_validations(changeset, &(&1))
    %{title: [format: ~r/pattern/, length: [min: 1, max: 20]]}

    iex> traverse_validations(changeset, fn
    ...>   {:length, opts} -> {:length, "#{Keyword.get(opts, :min, 0)}-#{Keyword.get(opts, :max, 32)}"}
    ...>   {:format, %Regex{source: source}} -> {:format, "/#{source}/"}
    ...>   {other, opts} -> {other, inspect(opts)}
    ...> end)
    %{title: [format: "/pattern/", length: "1-20"]}
