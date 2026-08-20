export const DynamicChild = (props) => (
  <div>
    <h1>{props.title}</h1>
    <p>Hello {props.name}</p>
    <span>{count()}</span>
  </div>
);
