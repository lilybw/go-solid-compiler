export const Namespaces = () => (
  <div
    on:CustomEvent={handler}
    prop:value={v()}
    attr:data-id={id()}
    style:color={color()}
    class:active={isOn()}
    ref={myRef}
  />
);
