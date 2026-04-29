/* globals React, ReactDOM, Desktop */

function App() {
  return (
    <div style={{position:"fixed", inset:0, background:"#0a0c12", overflow:"auto"}}>
      <div style={{margin:"24px auto", width:1440, height:900, boxShadow:"0 30px 80px rgba(0,0,0,0.6)", borderRadius:8, overflow:"hidden", position:"relative"}}>
        <Desktop variant="aurora"/>
        <NovaTweaks/>
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App/>);
