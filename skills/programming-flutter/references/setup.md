# Flutter Project Setup

Read this when you create a Flutter project or change its `analysis_options.yaml`.

`analysis_options.yaml`: start from the `sadensmol-programming-dart` block (its `references/setup.md`) with
`include: package:flutter_lints/flutter.yaml`, then add:

```yaml
analyzer:
  errors:
    missing_required_param: error
    missing_return: error

linter:
  rules:
    - avoid_print                       # use logger
    - avoid_unnecessary_containers
    - prefer_const_constructors_in_immutables
    - prefer_const_literals_to_create_immutables
    - sized_box_for_whitespace          # SizedBox over Container for spacing
    - sort_child_properties_last        # child: trailing
    - use_build_context_synchronously   # catches async + BuildContext bugs
    - use_key_in_widget_constructors
    - avoid_web_libraries_in_flutter
```
