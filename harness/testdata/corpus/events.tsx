export const Events = () => (
  <div>
    <button onClick={inc}>+</button>
    <input onInput={onInput} />
    <div onScroll={onScroll} />
  </div>
);
