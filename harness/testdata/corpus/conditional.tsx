export const Conditional = () => (
  <div>
    {cond() ? <a href="/y">yes</a> : <b>no</b>}
    {flag() && <span>maybe</span>}
  </div>
);
