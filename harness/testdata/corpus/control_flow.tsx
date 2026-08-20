export const ControlFlow = () => (
  <div>
    <For each={items()}>{(item) => <li>{item.name}</li>}</For>
    <Show when={ready()} fallback={<span>loading</span>}>
      <p>{body()}</p>
    </Show>
  </div>
);
