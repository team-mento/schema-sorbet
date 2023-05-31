# Schema to Sorbet

A command-line tool to convert well-formed schemas to [Sorbet](https://sorbet.org/) types.

This reduces hand-writing models when interacting with structured formats such as OpenAPI or JSON Schema and creates the corresponding Ruby + Sorbet types.

## OpenAPI

To convert OpenAPI documents to Sorbet types, you can run `openapi-sorbet`.

### Installation

```sh
go install gitlab.com/tanna.dev/schema-sorbet/cmd/openapi-sorbet@latest
```

### Usage

```sh
curl https://raw.githubusercontent.com/OAI/OpenAPI-Specification/main/examples/v3.0/petstore.yaml -Lo petstore.yaml

openapi-sorbet -path petstore.yaml -module ExternalClients::Petstore
```

This will output the following files for the `#!/components/schemas` in the OpenAPI spec:

`out/external_clients/petstore/pets.rb`:

```ruby
# typed: strict
# frozen_string_literal: true

=begin
Generated from OpenAPI specification for
  Swagger Petstore 1.0.0
using
  openapi-sorbet version (unknown).
DO NOT EDIT.
=end
 module ExternalClients
 module Petstore
=begin
Pets
=end
Pets = T.type_alias { T::Array[T.untyped]}
end
end
```

`out/external_clients/petstore/pet.rb`:

```ruby
# typed: strict
# frozen_string_literal: true

=begin
Generated from OpenAPI specification for
  Swagger Petstore 1.0.0
using
  openapi-sorbet version (unknown).
DO NOT EDIT.
=end
 module ExternalClients
 module Petstore
=begin
Pet
=end
class Pet  < T::Struct
extend T::Sig

const :id, Integer
const :name, String
const :tag, T.nilable(String)
end
end
end
```

`out/external_clients/petstore/error.rb`:

```ruby
# typed: strict
# frozen_string_literal: true

=begin
Generated from OpenAPI specification for
Swagger Petstore 1.0.0
using
openapi-sorbet version (unknown).
DO NOT EDIT.
=end
module ExternalClients
module Petstore
=begin
Error
=end
class Error  < T::Struct
extend T::Sig

const :code, Integer
const :message, String
end
end
end
```

**NOTE** that these are outputted un-formatted, and will need formatting through `rubocop` or `rubyfmt`.

## Licensing

This project is licensed under the Apache-2.0 license.
