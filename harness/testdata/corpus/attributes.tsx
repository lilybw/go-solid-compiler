export const Attributes = () => (
  <div>
    <a href={url()} title="static">link</a>
    <input value={val()} disabled />
    <div class={cls()} classList={{ active: isOn() }} style={styles()} />
  </div>
);
