export const Components = () => (
  <div>
    <Card title="static" count={count()} />
    <Wrapper>
      <span>child</span>
    </Wrapper>
    <UI.Button onClick={go}>Go</UI.Button>
  </div>
);
