// Copyright (C) 2026 Taras Mankovski. All rights reserved.
// This code is governed by the BSD license found in the LICENSE file.
/*---
esid: sec-generator.prototype.return
description: >
    Calling generator.return() before the generator has started (before any
    .next() call) returns immediately with done: true and does not enter
    the generator body.
info: |
    27.5.3.2 Generator.prototype.return ( value )
    
    1. Let g be the this value.
    2. Let C be Completion Record { [[Type]]: return, [[Value]]: value, [[Target]]: empty }.
    3. Return ? GeneratorResumeAbrupt(g, C, empty).
    
    When the generator is in "suspendedStart" state, GeneratorResumeAbrupt
    completes the generator without executing any of its body.
features: [generators]
---*/

var entered = false;

function* neverStarted() {
  entered = true;
  yield "work";
}

var gen = neverStarted();

// Call return() before any next() call
var result = gen.return("cancelled");

assert.sameValue(result.value, "cancelled", "return value passed through");
assert.sameValue(result.done, true, "generator completed immediately");
assert.sameValue(entered, false, "generator body was never entered");

// Subsequent calls should also return done: true
result = gen.next();
assert.sameValue(
  result.value,
  undefined,
  "subsequent next() returns undefined",
);
assert.sameValue(result.done, true, "generator remains completed");
