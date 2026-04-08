// lib.js — exports its own import.meta.url so callers can see where it lives.
export const libURL = import.meta.url;

export function greet(name) {
    return `Hello, ${name}! (from ${import.meta.url})`;
}
