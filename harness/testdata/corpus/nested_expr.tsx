export const NestedExpr = () => (
  <ul>
    {items().map((i) => (
      <li class={i.cls}>{i.label}</li>
    ))}
  </ul>
);
