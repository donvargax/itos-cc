Feature: Cyclomatic complexity
  A unit's complexity is 1 plus every decision it makes: each branch, loop,
  catch, case, and short-circuit operator, including those inside nested
  callbacks and closures, but not inside a route callback, which is a unit of
  its own.

  Scenario Outline: Straight-line code has complexity 1
    Given a <language> function with no branches
    When its complexity is measured
    Then it is 1

    Examples:
      | language   |
      | TypeScript |
      | Python     |
      | Go         |
      | Kotlin     |

  Scenario: TypeScript decisions
    Given the TypeScript function:
      """
      function branches(a: number, b?: string) {
        if (a > 0 && b) { return 1; } else if (a < 0 || !b) { return 2; }
        for (const x of [1]) {}
        while (a--) {}
        try { a++; } catch (e) {}
        const c = a ? 1 : 2;
        const d = b ?? "x";
        [1].forEach((n) => { if (n) {} });
        switch (a) { case 1: break; case 2: break; default: break; }
      }
      """
    When its complexity is measured
    Then it is 13
    # if, &&, else-if, ||, for, while, catch, ?:, ??, callback if, 2 cases

  Scenario: Optional chaining is a decision
    Given the TypeScript function:
      """
      export function choose(a, b, c) {
        return a ?? b?.c ?? c?.(1) ?? c?.[0];
      }
      """
    When its complexity is measured
    Then it is 7
    # 3 ??, and ?. on a property, a call, and an index
    And it is 7 in a .js file too
    And a Kotlin safe call a?.b counts like the elvis ?:

  Scenario: Python decisions
    Given the Python function:
      """
      def branches(a, b):
          if a and b:
              pass
          elif a or not b:
              pass
          for x in range(3):
              pass
          while a:
              a -= 1
          try:
              pass
          except ValueError:
              pass
          c = 1 if a else 2
          d = [x for x in range(3) if x]
          match a:
              case 1:
                  pass
              case _:
                  pass
      """
    When its complexity is measured
    Then it is 13
    # if, and, elif, or, for, while, except, if-else, comprehension for + if, 2 cases

  Scenario: Go decisions
    Given the Go function:
      """
      func branches(a int, ch chan int, v any) {
      	if a > 0 && a < 9 || a == 20 {
      	}
      	for i := 0; i < a; i++ {
      	}
      	switch a {
      	case 1:
      	case 2:
      	default:
      	}
      	switch v.(type) {
      	case int:
      	}
      	select {
      	case <-ch:
      	default:
      	}
      	f := func() { if a > 0 {} }
      	f()
      }
      """
    When its complexity is measured
    Then it is 10
    # if, &&, ||, for, 2 cases, type case, comm case, closure if

  Scenario: Kotlin decisions
    Given the Kotlin function:
      """
      fun branches(a: Int, b: String?) {
          if (a > 0 && b != null || a < -5) { }
          for (x in 1..3) { }
          while (false) { }
          do { } while (false)
          try { } catch (e: Exception) { }
          val c = b ?: "x"
          when (a) {
              1 -> {}
              2, 3 -> {}
              else -> {}
          }
          listOf(1).forEach { if (it > 0) { } }
      }
      """
    When its complexity is measured
    Then it is 12
    # if, &&, ||, for, while, do-while, catch, ?:, 2 when entries, lambda if
