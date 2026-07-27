import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import "./style.css";
import { Sketch } from "./Sketch.jsx";

const SAVE_AFTER_IDLE_MS = 800;

/** Where this book is mounted: "/" when it is alone, "/arbay/" when it is one of several. */
export const base = location.pathname.replace(/[^/]*$/, "");

async function api(path, options) {
  const response = await fetch(base + path, options);
  if (!response.ok) throw new Error(`${options?.method ?? "GET"} ${path}: ${response.status}`);
  return response.json();
}

/** The hash is the address: #/ , #/view/results , #/sketch/results , #/markets . */
function useRoute() {
  const [route, setRoute] = useState(() => location.hash.slice(2) || "");
  useEffect(() => {
    const follow = () => setRoute(location.hash.slice(2) || "");
    window.addEventListener("hashchange", follow);
    return () => window.removeEventListener("hashchange", follow);
  }, []);
  return route;
}

const slug = (uid) => uid.replace(/^VIEW-/, "").toLowerCase();
const statusClass = (status) => `pill ${status.toLowerCase()}`;

/**
 * Light or dark, which is the reader's to decide.
 *
 * Nothing is stored until someone chooses, so a screen that is already dark
 * stays dark without being asked.
 */
function useTheme() {
  const [chosen, setChosen] = useState(() => localStorage.getItem("viewbook.theme") || "");
  const screen = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  const showing = chosen || screen;

  useEffect(() => {
    if (chosen) document.documentElement.setAttribute("data-theme", chosen);
    else document.documentElement.removeAttribute("data-theme");
  }, [chosen]);

  const flip = () => {
    const next = showing === "dark" ? "light" : "dark";
    localStorage.setItem("viewbook.theme", next);
    setChosen(next);
  };
  return { showing, flip };
}

/**
 * The command that makes this project's renders, and what it is saying.
 *
 * A build that prints nothing for four minutes is indistinguishable from one
 * that has hung, so while it runs the page reads its output every second.
 */
function useRenders() {
  const [state, setState] = useState(null);
  const read = useCallback(
    () => api("api/renders").then(setState).catch(() => {}), []);

  useEffect(() => { read(); }, [read]);
  useEffect(() => {
    if (!state?.running) return undefined;
    const every = setInterval(read, 1000);
    return () => clearInterval(every);
  }, [state?.running, read]);

  const start = () => api("api/renders", { method: "POST" }).then(setState)
    .catch((error) => setState({ ...state, failed: error.message }));
  const stop = () => api("api/renders", { method: "DELETE" }).then(setState).catch(() => {});
  return { ...state, start, stop };
}

function App() {
  const route = useRoute();
  const [model, setModel] = useState(null);
  const [config, setConfig] = useState(null);
  const [saved, setSaved] = useState("");
  // Renders are files, and a browser holds on to a file it has already seen.
  // This changes with them, so what is on screen is what is on disk.
  const [stamp, setStamp] = useState(0);
  const timer = useRef(null);
  const renders = useRenders();
  const theme = useTheme();
  // Which interface this page is running. When the server starts serving a
  // different one, this page is the old one and says so by reloading.
  const running = useRef(null);

  const reload = useCallback(() => {
    api("api/model").then(setModel);
    api("api/config").then(setConfig);
  }, []);

  useEffect(() => { reload(); }, [reload]);

  useEffect(() => {
    const version = config?.interface;
    if (!version) return;
    if (running.current === null) running.current = version;
    else if (running.current !== version) location.reload();
  }, [config?.interface]);

  // The files are the document and the agent writes to them too, so the page
  // follows them rather than showing whatever it read when it opened.
  //
  // ?static=1 skips it: an open stream never lets a page finish loading, and a
  // screenshot tool waits for exactly that.
  useEffect(() => {
    if (new URLSearchParams(location.search).has("static")) return undefined;
    const stream = new EventSource(base + "api/events");
    // A file changed under the page, so the page shows the new one. Saying so
    // in the corner tells nobody anything they cannot see.
    stream.addEventListener("changed", () => {
      reload();
      setStamp(Date.now());
    });
    return () => stream.close();
  }, [reload]);

  // The model on disk is the document; an edit here is written back after a pause.
  const save = useCallback((next, announce = false) => {
    setModel(next);
    setSaved("saving…");
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      api("api/model", {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          ...(announce ? { "X-Viewbook-Announce": "1" } : {}),
        },
        // Written to the file as sent, so a changed note is a changed line.
        body: JSON.stringify(next, null, 2),
      })
        .then(() => setSaved("saved"))
        .catch((error) => setSaved(`not saved: ${error.message}`));
    }, SAVE_AFTER_IDLE_MS);
  }, []);

  if (!model || !config) return <div className="loading">Reading the model…</div>;

  const views = model.views;
  const [section, argument] = route.split("/");

  let page;
  if (section === "view") {
    const view = views.find((v) => slug(v.uid) === argument);
    page = view
      ? <ViewPage
          model={model}
          view={view}
          onChange={save}
          stamp={stamp}
          theme={theme.showing}
          required={config.states}
        />
      : <NoSuchView />;
  } else if (section === "sketch") {
    page = <Sketch name={argument} title={views.find((v) => slug(v.uid) === argument)?.title} />;
  } else if (section === "table") {
    const table = config.tables.find((t) => t.name === argument);
    page = table ? <TablePage table={table} /> : <NoSuchView what="table" />;
  } else {
    page = (
      <IndexPage
        views={views}
        model={model}
        stamp={stamp}
        renders={renders}
        theme={theme.showing}
        required={config.states}
      />
    );
  }

  // One bar across the top rather than a column down the side: a screen in
  // landscape has width to spare and height to save, and a phone has neither to
  // give to a permanent list.
  //
  // Viewbook is the tool and leads out to every book it serves; the name beside
  // it is which book this is, and leads to that book's index.
  return (
    <div className="shell">
      <header className="bar">
        <span className="brand">
          {/* A book named after the tool is not named twice. */}
          {config.title.toLowerCase() !== "viewbook" && <span className="tool">Viewbook</span>}
          <a className={`book ${route === "" ? "on" : ""}`} href="#/">{config.title}</a>
        </span>
        <nav className="tabs">
          {views.map((view) => (
            <a
              key={view.uid}
              className={section === "view" && argument === slug(view.uid) ? "on" : ""}
              href={`#/view/${slug(view.uid)}`}
            >
              {view.title}
              {view.status === "Missing" && <span className="dot" title="not built" />}
            </a>
          ))}
          {config.tables.map((table) => (
            <a
              key={table.name}
              className={`table-tab ${section === "table" && argument === table.name ? "on" : ""}`}
              href={`#/table/${table.name}`}
            >
              {table.title}
            </a>
          ))}
        </nav>
        {renders.running && (
          <a className="running" href="#/" title="the renders are being made">making renders</a>
        )}
        {/* The way out of this book, which the book's own name is not: it leads
            to this book's index. A book served alone has nowhere else to go. */}
        {base !== "/" && <a className="books" href="/">all books</a>}
        <button
          className="theme"
          onClick={theme.flip}
          title={`showing ${theme.showing}; click for ${theme.showing === "dark" ? "light" : "dark"}`}
        >
          {theme.showing === "dark" ? "☾" : "☀"}
        </button>
        <span className="state">{saved}</span>
      </header>
      <main>{page}</main>
    </div>
  );
}

function NoSuchView({ what = "view" }) {
  return <div className="page"><h1>No such {what}</h1></div>;
}

function requirementsOf(model, uid) {
  const own = model.requirements.filter((r) =>
    r.relations.some((x) => x.to === uid && x.role === "Lives in"));
  return {
    all: own,
    built: own.filter((r) => r.status === "Built").length,
    broken: own.filter((r) => r.status === "Broken").length,
    missing: own.filter((r) => r.status === "Missing").length,
  };
}

/**
 * What a view looks like today, at whatever shape it actually is.
 *
 * The same screen is upright on a phone and wide on a desktop, and a view can
 * carry both. Which one an image is, is measured when it loads rather than
 * declared in the model, so a project only has to drop the file in img/.
 */
/**
 * Every state a view can be in, and what each looks like.
 *
 * A screen is not one picture. It is full and it is empty, it is loading and it
 * has failed, it is a phone held upright and a window three times as wide. A
 * book that shows only the happy one is a book that lies by omission, so the
 * states a project has not rendered are shown as gaps rather than left out.
 */
function statesOf(model, view, required) {
  const own = model.states.filter((state) =>
    state.relations?.some((r) => r.to === view.uid && r.role === "State of"));

  const asItIs = { uid: `${view.uid}-AS-IT-IS`, title: "As it is", shots: rendersOf(view) };
  const shown = own.map((state) => ({
    uid: state.uid,
    title: state.title,
    statement: state.statement,
    shots: rendersOf(state),
  }));

  // A state the project promised in its config but has not modelled at all is
  // still a state this view can be in, and the gap is the point.
  const named = new Set(shown.map((state) => state.title.toLowerCase()));
  const missing = (required ?? [])
    .filter((title) => !named.has(String(title).toLowerCase()))
    .map((title) => ({ uid: `${view.uid}-${title}`, title, shots: [], promised: true }));

  return [asItIs, ...shown, ...missing].filter(
    (state, index) => index === 0 || state.shots.length > 0 || state.promised);
}

function Render({ model, view, onShowing, stamp, theme, required }) {
  const states = statesOf(model, view, required);
  const [state, setState] = useState(0);
  const [chosen, setChosen] = useState(0);

  const here = states[Math.min(state, states.length - 1)];
  const shots = here.shots;

  useEffect(() => { setState(0); }, [view.uid]);
  useEffect(() => { setChosen(preferred(shots, theme)); }, [view.uid, state, theme]);

  const tell = (which) => onShowing({
    state: here.title,
    named: which ? (which.label ?? which.file.replace(/\.[a-z]+$/, "")) : "",
  });

  const shot = shots[Math.min(chosen, shots.length - 1)];
  return (
    <>
      {shot ? (
        <div className="frame">
          <img
            src={`${base}img/small/${shot.file}${stamp ? `?v=${stamp}` : ""}`}
            alt={`${view.title}, ${here.title}`}
            onLoad={(e) => onShowing({
              shape: e.target.naturalWidth > e.target.naturalHeight ? "landscape" : "portrait",
              state: here.title,
              named: shot.label ?? shot.file.replace(/\.[a-z]+$/, ""),
            })}
          />
        </div>
      ) : (
        <div className="noshot tall">
          <span>
            Nothing renders {here === states[0] ? "this view" : `this view ${here.title.toLowerCase()}`} yet.
            {here.statement && <><br />{here.statement}</>}
          </span>
        </div>
      )}

      <div className="shapes">
        {states.map((one, index) => (
          <button
            key={one.uid}
            className={`${index === state ? "on" : ""} ${one.shots.length === 0 ? "gap" : ""}`}
            title={one.shots.length === 0 ? "no render of this state yet" : one.statement}
            onClick={() => { setState(index); tell(one.shots[preferred(one.shots, theme)]); }}
          >
            {one.title}
          </button>
        ))}
      </div>

      {shots.length > 1 && (
        <div className="shapes">
          {shots.map((one, index) => (
            <button
              key={one.file}
              className={`shape ${index === chosen ? "on" : ""}`}
              onClick={() => { setChosen(index); tell(one); }}
            >
              {one.label ?? one.file.replace(/\.[a-z]+$/, "")}
            </button>
          ))}
        </div>
      )}
    </>
  );
}

/** Every render a view carries: one screenshot, or a list of them. */
function rendersOf(view) {
  if (Array.isArray(view.renders) && view.renders.length > 0) {
    return view.renders.map((one) => (typeof one === "string" ? { file: one } : one));
  }
  return view.screenshot ? [{ file: view.screenshot }] : [];
}

/**
 * The render to show first when a view carries several.
 *
 * A project that ships a screen drawn light and the same screen drawn dark says
 * so in the file name or the label, and the page shows the one that matches what
 * the reader is looking at rather than a grid of screenshots in two themes.
 */
function preferred(shots, theme) {
  const says = (shot) => `${shot.label ?? ""} ${shot.file}`.toLowerCase().includes(theme);
  const matching = shots.findIndex(says);
  // No render says which theme it is, so the first one stands. Guessing by
  // elimination would swap an upright render for a wide one, which answers a
  // question nobody asked.
  return matching >= 0 ? matching : 0;
}

/** A thumbnail, cropped when it is a tall screen and shown whole when it is wide. */
function Thumb({ view, stamp, theme }) {
  const [shape, setShape] = useState("portrait");
  const shots = rendersOf(view);
  if (shots.length === 0) return <div className="noshot">nothing renders this yet</div>;
  const shot = shots[preferred(shots, theme)];
  return (
    <img
      className={shape}
      src={`${base}img/card/${shot.file}${stamp ? `?v=${stamp}` : ""}`}
      alt={view.title}
      loading="lazy"
      onLoad={(e) => setShape(e.target.naturalWidth > e.target.naturalHeight ? "landscape" : "portrait")}
    />
  );
}

function IndexPage({ views, model, stamp, renders, theme, required }) {
  const open = model.requirements.filter((r) => r.status !== "Built");
  const gaps = views.reduce(
    (count, view) => count + statesOf(model, view, required).filter((s) => s.shots.length === 0).length, 0);
  return (
    <div className="page">
      <header className="page-head">
        <div>
          <h1>Every view</h1>
          <p>
            {views.length} views · {model.requirements.length} requirements ·{" "}
            {open.length === 0 ? "all built" : `${open.length} not built`}
            {gaps > 0 && <> · <span className="gapcount">{gaps} states with no render</span></>}
          </p>
        </div>
      </header>
      <div className="grid">
        {views.map((view) => {
          const own = requirementsOf(model, view.uid);
          return (
            <a className="card" key={view.uid} href={`#/view/${slug(view.uid)}`}>
              <div className="shot">
                <Thumb view={view} stamp={stamp} theme={theme} />
              </div>
              <div className="card-body">
                <h2>{view.title}</h2>
                <p>{view.statement.split(".")[0]}.</p>
                <div className="counts">
                  {statesOf(model, view, required).filter((s) => s.shots.length === 0).length > 0 && (
                    <span className="pill missing">
                      {statesOf(model, view, required).filter((s) => s.shots.length === 0).length} not rendered
                    </span>
                  )}
                  {own.built > 0 && <span className="pill built">{own.built} built</span>}
                  {own.broken > 0 && <span className="pill broken">{own.broken} broken</span>}
                  {own.missing > 0 && <span className="pill missing">{own.missing} missing</span>}
                </div>
              </div>
            </a>
          );
        })}
      </div>
      <RenderRun renders={renders} />
      <SketchBox />
    </div>
  );
}

/**
 * Making the renders again, from here.
 */
function RenderRun({ renders }) {
  const tail = useRef(null);
  useEffect(() => {
    if (tail.current) tail.current.scrollTop = tail.current.scrollHeight;
  }, [renders.output]);

  if (!renders.declared) return null;
  return (
    <section className="renders">
      <h3>The renders</h3>
      <div className="sketch-row">
        {renders.running ? (
          <>
            <button className="quiet" onClick={renders.stop}>Stop</button>
            <span className="hint">{renders.command}</span>
          </>
        ) : (
          <>
            <button className="send" onClick={renders.start}>Make them again</button>
            <span className="hint">
              {renders.statement || renders.command}
              {renders.took && ` · last run took ${renders.took}`}
              {renders.failed && ` · failed: ${renders.failed}`}
            </span>
          </>
        )}
      </div>
      {renders.output && <pre className="reply" ref={tail}>{renders.output}</pre>}
    </section>
  );
}

/**
 * Sketching, which is for a screen that does not exist yet.
 *
 * It sits with the index rather than on a view: a view that already renders has
 * a render, and drawing over it says nothing the screenshot does not.
 */
function SketchBox() {
  const [drawn, setDrawn] = useState([]);
  const [name, setName] = useState("");
  useEffect(() => {
    api("api/sketches").then(setDrawn).catch(() => setDrawn([]));
  }, []);

  const start = () => {
    const called = name.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
    if (called) location.hash = `#/sketch/${called}`;
  };

  return (
    <section className="sketches">
      <h3>Sketches</h3>
      <div className="sketch-row">
        {drawn.map((one) => (
          <a className="button" key={one} href={`#/sketch/${one}`}>{one}</a>
        ))}
        <input
          value={name}
          placeholder="a screen that does not exist yet"
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && start()}
        />
        <button className="send" onClick={start} disabled={!name.trim()}>Sketch it</button>
      </div>
    </section>
  );
}

function ViewPage({ model, view, onChange, stamp, theme, required }) {
  const [showing, setShowing] = useState({ shape: "portrait", named: "", state: "" });
  const reachedFrom = view.relations
    .filter((r) => r.role === "Reached from")
    .map((r) => model.views.find((v) => v.uid === r.to))
    .filter(Boolean);

  // The whole page is the view as it renders and the conversation about it. The
  // navigation already says which view this is, and what it has to do is
  // counted on the index; repeating either here only pushed the two things
  // someone came for off the screen.
  return (
    <div className="page">
      <div className={`work ${showing.shape}`}>
        <section className="render">
          <Render
            model={model}
            view={view}
            required={required}
            onShowing={(next) => setShowing((was) => ({ ...was, ...next }))}
            stamp={stamp}
            theme={theme}
          />
          {reachedFrom.length > 0 && (
            <p className="from">
              Reached from{" "}
              {reachedFrom.map((v, i) => (
                <React.Fragment key={v.uid}>
                  {i > 0 && ", "}
                  <a href={`#/view/${slug(v.uid)}`}>{v.title}</a>
                </React.Fragment>
              ))}
            </p>
          )}
        </section>
        {/* What is being looked at goes with the question: which render, and
            whether the page is light or dark, are half of what "this looks
            wrong" means. */}
        <Ask
          about={view.title}
          looking={[showing.state, showing.named, theme].filter(Boolean).join(", ")}
        />
      </div>
    </div>
  );
}

/**
 * A table a project declares in viewbook.json: where the rows come from, which columns to show,
 * and how to say a value that is a list or a flag. Nothing here knows what the rows are about.
 */
/**
 * Say something to the session, and see it land.
 *
 * The page had a notes box that saved silently, so asking for something looked
 * exactly like doing nothing: no button, no confirmation, no reply. This is the
 * other half of the loop - what is typed goes to the conversation, and what the
 * conversation says comes back underneath.
 */
function Ask({ about, looking }) {
  const [text, setText] = useState("");
  const [shots, setShots] = useState([]);
  const [state, setState] = useState("");
  const [reply, setReply] = useState("");
  const talk = useRef(null);
  const box = useRef(null);

  // The composer is as tall as what has been typed, up to a third of the
  // screen: a fixed box scrolls its own three lines while the page beneath it
  // stays empty, which is the worst of both.
  const grow = () => {
    const field = box.current;
    if (!field) return;
    const cap = window.innerHeight * 0.34;
    field.style.height = "auto";
    field.style.height = `${Math.min(field.scrollHeight, cap)}px`;
    // A scrollbar around text that fits is furniture; it appears only at the cap.
    field.style.overflowY = field.scrollHeight > cap ? "auto" : "hidden";
  };
  useEffect(grow, [text]);
  // The conversation is there whether or not this page asked the last question,
  // so it is shown on arrival rather than only after sending something.


  const send = () => {
    const message = text.trim();
    if (!message && shots.length === 0) return;
    setState("sending…");
    // A pasted image is already a file in the project; the conversation is given
    // its path, which is something it can actually open.
    const attached = shots.map((s) => `\n${s.path}`).join("");
    // Which render is on screen, and whether it is light or dark, travels with
    // the message: otherwise the answer is about a picture nobody is looking at.
    const at = about ? `About ${about}${looking ? ` (${looking})` : ""}: ` : "";
    const said = at + message + attached;
    api("api/say", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: said }),
    })
      .then(() => {
        setState("sent to the session");
        setText("");
        setShots([]);
      })
      .catch((error) => setState(`not sent: ${error.message}`));
  };

  const paste = (event) => {
    const images = [...(event.clipboardData?.items ?? [])]
      .filter((item) => item.type.startsWith("image/"));
    if (images.length === 0) return;
    event.preventDefault();
    setState("keeping the image…");
    images.forEach((item) => {
      const file = item.getAsFile();
      if (!file) return;
      fetch(base + "api/paste", {
        method: "POST",
        headers: { "Content-Type": file.type },
        body: file,
      })
        .then((r) => r.json())
        .then((kept) => {
          setShots((now) => [...now, kept]);
          setState("image kept; it goes with the message");
        })
        .catch((error) => setState(`image not kept: ${error.message}`));
    });
  };

  // The newest line is the one being waited for, so the pane stays at its end.
  useEffect(() => {
    if (talk.current) talk.current.scrollTop = talk.current.scrollHeight;
  }, [reply]);

  // After saying something, follow the session for a while so the answer shows
  // up here rather than only in a terminal somewhere.
  useEffect(() => {
    const read = () => api("api/session").then((r) => setReply(r.text || "")).catch(() => {});
    read();
    const every = setInterval(read, 2500);
    return () => clearInterval(every);
  }, []);

  // The conversation stands beside the render, because those two and what is
  // typed between them are what the page is for.
  return (
    <section className="talk">
      {reply
        ? <pre className="reply" ref={talk}>{reply.trimEnd().split("\n").slice(-80).join("\n")}</pre>
        : <p className="hint">Nothing in the session yet.</p>}
      <div className="dock-row">
      <textarea
        className="ask"
        ref={box}
        rows={1}
        value={text}
        placeholder={`Ask about ${about ?? "this"}`}
        onChange={(e) => { setText(e.target.value); grow(); }}
        onPaste={paste}
        // Enter sends, because this is a message rather than a document.
        // Shift+Enter is the newline.
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
          }
        }}
      />
      {shots.length > 0 && (
        <div className="shots">
          {shots.map((shot) => (
            <span key={shot.url}>
              <img src={base + shot.url} alt="pasted" />
              <button className="quiet" onClick={() => setShots((now) => now.filter((s) => s !== shot))}>
                remove
              </button>
            </span>
          ))}
        </div>
      )}
        <button
          className="send"
          onClick={send}
          disabled={!text.trim() && shots.length === 0}
        >
          Send
        </button>
      </div>
      <div className="ask-bar">
        <span className={state.startsWith("not sent") ? "bad" : ""}>{state}</span>
      </div>
    </section>
  );
}

function TablePage({ table }) {
  const [data, setData] = useState(null);
  useEffect(() => {
    setData(null);
    api(`api/table/${table.name}`).then(setData).catch(() => setData({}));
  }, [table.name]);

  if (!data) return <div className="page"><h1>{table.title}</h1><p>Reading…</p></div>;

  const rows = table.rows ? (data[table.rows] ?? []) : (Array.isArray(data) ? data : []);
  const sorted = table.sortBy
    ? [...rows].sort((a, b) => String(a[table.sortBy]).localeCompare(String(b[table.sortBy])))
    : rows;

  const cell = (row, column) => {
    const value = row[column.field];
    if (Array.isArray(value)) return value.length ? value.join(", ").toLowerCase() : "—";
    if (typeof value === "boolean") return value ? (column.yes ?? "yes") : (column.no ?? "—");
    if (value === null || value === undefined || value === "") return column.empty ?? "—";
    return String(value);
  };

  return (
    <div className="page">
      <header className="page-head">
        <div>
          <h1>{table.title}</h1>
          {table.statement && <p>{table.statement}</p>}
        </div>
      </header>
      {/* A table wider than the screen scrolls on its own, rather than making
          the whole page slide sideways. */}
      <div className="table-scroll">
      <table className="table">
        <thead>
          <tr>{table.columns.map((c) => <th key={c.field}>{c.title}</th>)}</tr>
        </thead>
        <tbody>
          {sorted.map((row, index) => (
            <tr key={row.id ?? index}>
              {table.columns.map((column, position) => (
                <td key={column.field}>
                  {position === 0 ? <strong>{cell(row, column)}</strong> : cell(row, column)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      </div>
    </div>
  );
}

createRoot(document.getElementById("root")).render(<App />);
