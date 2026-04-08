// counter.js — a simple stateful class exported as a named binding.
export class Counter {
    #value;

    constructor(initial = 0) {
        this.#value = initial;
    }

    increment(by = 1) { this.#value += by; return this; }
    decrement(by = 1) { this.#value -= by; return this; }
    reset()           { this.#value = 0;   return this; }

    get value() { return this.#value; }

    toString() { return `Counter(${this.#value})`; }
}
