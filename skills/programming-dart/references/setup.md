# Dart Project Setup

Read this when you create a Dart project or change its `analysis_options.yaml`.

- Enable strict lints in `analysis_options.yaml` (Flutter projects add the Flutter rules from `sadensmol-programming-flutter`):

```yaml
include: package:lints/recommended.yaml   # or package:flutter_lints/flutter.yaml

analyzer:
  language:
    strict-casts: true
    strict-inference: true
    strict-raw-types: true

linter:
  rules:
    - always_declare_return_types
    - avoid_dynamic_calls
    - prefer_const_constructors
    - prefer_const_declarations
    - prefer_final_locals
    - unawaited_futures
    - use_super_parameters
```
