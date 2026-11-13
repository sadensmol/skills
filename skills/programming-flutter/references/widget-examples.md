# Flutter Widget Examples

Read this when you need a worked example of widget composition, `const` usage, `build()` discipline or the `StatefulWidget` lifecycle.

## Widget Composition

```dart
// Good — small, const, explicit inputs
class PriceTag extends StatelessWidget {
  const PriceTag({
    required this.label,
    required this.amount,
    this.highlighted = false,
    super.key,
  });

  final String label;
  final Money amount;
  final bool highlighted;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.all(8),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(label, style: theme.textTheme.bodyMedium),
          const SizedBox(width: 8),
          Text(
            amount.format(),
            style: theme.textTheme.titleMedium?.copyWith(
              color: highlighted ? theme.colorScheme.primary : null,
            ),
          ),
        ],
      ),
    );
  }
}
```

## `const` Discipline

```dart
// Good
return const Padding(
  padding: EdgeInsets.all(8),
  child: Text('Ready'),
);

// Good — whole list of const children
Column(
  children: const [
    Text('One'),
    SizedBox(height: 8),
    Text('Two'),
  ],
)
```

## `build()` Method Discipline

```dart
// Bad
@override
Widget build(BuildContext context) {
  final user = ApiClient().fetchUser(id);        // ❌ I/O in build
  return Text('${user.name}');
}

// Good
@override
void initState() {
  super.initState();
  _userFuture = widget.api.fetchUser(widget.id);
}

@override
Widget build(BuildContext context) {
  return FutureBuilder<User>(
    future: _userFuture,
    builder: (context, snap) => Text(snap.data?.name ?? '…'),
  );
}
```

## Lifecycle (`StatefulWidget`)

```dart
class _FeedState extends State<Feed> {
  late final StreamSubscription<FeedEvent> _sub;

  @override
  void initState() {
    super.initState();
    // One-time init. Widget is mounted but not yet laid out.
    // Safe: constructor args via widget.xxx, context (limited — no MediaQuery).
    _sub = widget.feed.events.listen(_onEvent);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    // Runs after initState and whenever an InheritedWidget we depend on changes.
    // Safe place to read Theme.of, MediaQuery.of, Provider.of(listen: true).
  }

  @override
  void didUpdateWidget(covariant Feed old) {
    super.didUpdateWidget(old);
    // Parent rebuilt and passed new widget fields.
    // Use this to react to changed constructor args (e.g., restart a stream).
    if (widget.feedId != old.feedId) {
      _sub.cancel();
      _sub = widget.feed.events.listen(_onEvent);
    }
  }

  @override
  void dispose() {
    _sub.cancel();
    super.dispose();   // super LAST in dispose
  }
}
```

## Keys — When and Which

```dart
// Good — ValueKey on list items that can reorder
return ListView(
  children: [
    for (final task in tasks)
      TaskTile(key: ValueKey(task.id), task: task),
  ],
);
```
