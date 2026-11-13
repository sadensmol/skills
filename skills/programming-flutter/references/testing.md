# Flutter Testing

Read this when you write or change unit, widget, golden or integration tests.

## Testing

Pyramid:

```
       /\
      /  \   Few integration tests (happy-path E2E)
     /----\
    /      \  Some widget tests (screen-level)
   /--------\
  /          \ Many unit tests (VMs, repositories, utils)
 /____________\
```

### Unit tests — ViewModels, repositories, pure logic

```dart
void main() {
  group('LoginViewModel', () {
    late FakeAuthRepo repo;
    late LoginViewModel vm;

    setUp(() {
      repo = FakeAuthRepo();
      vm = LoginViewModel(repo);
    });

    test('emits loading then success on valid credentials', () async {
      repo.nextUser = const User(id: '1', name: 'Ada');

      final states = <LoginState>[];
      vm.addListener(() => states.add(vm.state));

      await vm.submit(email: 'a@b', password: 'pw12345678');

      expect(states.first, isA<_Loading>());
      expect(states.last, isA<_Success>());
    });
  });
}
```

### Widget tests — screens and complex widgets

```dart
void main() {
  testWidgets('shows error when login fails', (tester) async {
    final repo = FakeAuthRepo()..nextError = const AuthException('bad creds');
    await tester.pumpWidget(
      MaterialApp(
        home: ChangeNotifierProvider(
          create: (_) => LoginViewModel(repo),
          child: const LoginScreen(),
        ),
      ),
    );

    await tester.enterText(find.byKey(const ValueKey('email')), 'a@b');
    await tester.enterText(find.byKey(const ValueKey('password')), 'pw12345678');
    await tester.tap(find.byKey(const ValueKey('submit')));
    await tester.pump();                 // loading frame
    await tester.pump(const Duration(seconds: 1));

    expect(find.text('bad creds'), findsOneWidget);
  });
}
```

Rules:
- **`pumpWidget`** → **`pump`** after every async → **`pumpAndSettle`** *only* for finite animations. Don't use `pumpAndSettle` on indeterminate progress spinners — it hangs.
- **Use `ValueKey` on things you test** so tests don't depend on visual text.
- **Wrap with `MaterialApp`** (or `WidgetsApp`) when the widget reads `Theme`/`Directionality`/`MediaQuery`.
- **Fake at the repository boundary**, not deep inside services — tests stay realistic.

### Golden tests

Catch visual regressions. Run with `flutter test --update-goldens` to refresh when intentional.

```dart
testWidgets('PriceTag matches golden', (tester) async {
  await tester.pumpWidget(const MaterialApp(
    home: Scaffold(body: Center(child: PriceTag(label: 'Total', amount: Money(42)))),
  ));
  await expectLater(find.byType(PriceTag), matchesGoldenFile('price_tag.png'));
});
```

CI: run goldens on a **single fixed host** (Linux or a specific CI image) — font rendering differs by OS and will false-positive otherwise.

### Integration tests

One per critical user journey (login → main screen → key action). Run on CI with `flutter test integration_test/`. Keep them few; they're slow and flaky compared to widget tests.
