// math.js — a small library of pure functions exported as named bindings.
export function add(a, b)      { return a + b; }
export function subtract(a, b) { return a - b; }
export function multiply(a, b) { return a * b; }

export const PI = 3.141592653589793;

export function circleArea(r) { return PI * r * r; }
