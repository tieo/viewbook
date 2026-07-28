import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import "./style.css";
import { Group, Panel, Separator } from "react-resizable-panels";
import { Sketch } from "./Sketch.jsx";

const SAVE_AFTER_IDLE_MS = 800;

/** Where this book is mounted: "/" when it is alone, "/arbay/" when it is one of several. */
export const base = location.pathname.replace(/[^/]*$/, "");

async function api(path, options) {
  const response = await fetch(base + path, options);
  // Redirected to a sign-in page: the session behind the tunnel expired, and
  // whatever comes back is a login form rather than an answer.
  if (response.redirected && !response.url.startsWith(location.origin)) {
    throw new Error("signed out; reload the page to sign in again");
  }
  if (!response.ok) {
    // The server says what went wrong; a status code says only that something
    // did, which is the least useful half of the answer.
    let said = "";
    try {
      said = (await response.json())?.error ?? "";
    } catch {
      said = "";
    }
    throw new Error(said || `${options?.method ?? "GET"} ${path}: ${response.status}`);
  }
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

/** Whether the window has room to show every shape of a screen side by side. */
function useRoomy() {
  const query = "(min-width: 1000px) and (orientation: landscape)";
  const [roomy, setRoomy] = useState(() => window.matchMedia(query).matches);
  useEffect(() => {
    const watch = window.matchMedia(query);
    const follow = () => setRoomy(watch.matches);
    watch.addEventListener("change", follow);
    return () => watch.removeEventListener("change", follow);
  }, []);
  return roomy;
}

const slug = (uid) => uid.replace(/^VIEW-/, "").toLowerCase();

/**
 * Light or dark, which is the reader's to decide.
 *
 * Nothing is stored until someone chooses, so a screen that is already dark
 * stays dark without being asked.
 */
function useTheme() {
  // ?theme=light or dark holds the page in one, which is how a project renders
  // both looks of the same screen.
  const asked = new URLSearchParams(location.search).get("theme") || "";
  const [chosen, setChosen] = useState(() => asked || localStorage.getItem("viewbook.theme") || "");
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
/** What is wrong with this book: a model that will not parse, states nothing renders. */
function useCheck(stamp) {
  const [trouble, setTrouble] = useState({ trouble: [], gaps: [] });
  useEffect(() => {
    api("api/check").then(setTrouble).catch(() => {});
  }, [stamp]);
  return trouble;
}

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
  const [broken, setBroken] = useState("");
  // What is half-typed survives a change of view. It is a draft, and the only
  // thing that sends it is sending it.
  const [drafts, setDrafts] = useState({});
  // A screenshot of a state needs the state to happen on demand. This is how
  // viewbook renders its own: ?showing=loading, empty or failed.
  const forced = new URLSearchParams(location.search).get("showing") || "";
  const timer = useRef(null);
  const renders = useRenders();
  const theme = useTheme();
  // Which interface this page is running. When the server starts serving a
  // different one, this page is the old one and says so by reloading.
  const running = useRef(null);

  const reload = useCallback(() => {
    api("api/model").then(setModel).catch((error) => setBroken(error.message));
    api("api/config").then(setConfig).catch((error) => setBroken(error.message));
  }, []);

  useEffect(() => { reload(); }, [reload]);

  useEffect(() => {
    // Opening a book is asking to work on the project, so whatever works on it
    // is started rather than waited for.
    api("api/session", { method: "POST" }).catch(() => {});
  }, []);

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

  if (forced === "loading") return <div className="loading">Reading the model…</div>;
  if (forced === "failed" || (broken && (!model || !config))) {
    return (
      <div className="loading broken">
        <p>This book&rsquo;s model could not be read.</p>
        <p className="why">{broken || "model.json is not there, or is not a model"}</p>
      </div>
    );
  }
  if (!model || !config) return <div className="loading">Reading the model…</div>;

  const views = forced === "empty" ? [] : model.views;
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
          draft={drafts[view.uid] ?? ""}
          onDraft={(text) => setDrafts((was) => ({ ...was, [view.uid]: text }))}
          forced={forced}
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
          {/* Viewbook is the tool and leads to every book it serves. A book
              named after it is not named twice. */}
          <a className="tool" href="/">Viewbook</a>
          {config.title.toLowerCase() !== "viewbook" && (
            <span className="book">{config.title}</span>
          )}
        </span>
        <nav className="tabs">
          <a className={route === "" ? "on" : ""} href="#/">Overview</a>
          <span className="divider" aria-hidden="true" />
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
          {config.tables.length > 0 && <span className="divider" aria-hidden="true" />}
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
        <span className="state">{saved}</span>
        <button
          className="theme"
          onClick={theme.flip}
          title={`showing ${theme.showing}; click for ${theme.showing === "dark" ? "light" : "dark"}`}
        >
          {theme.showing === "dark" ? "☾" : "☀"}
        </button>
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

  const asItIs = { uid: `${view.uid}-DEFAULT`, title: "Default", shots: rendersOf(view) };
  const shown = own.map((state) => ({
    uid: state.uid,
    title: state.title,
    kind: state.kind,
    statement: state.statement,
    shots: rendersOf(state),
  }));

  // A state the project promised in its config but has not modelled at all is
  // still a state this view can be in, and the gap is the point.
  const named = new Set(shown.flatMap((state) =>
    [state.title, state.kind].filter(Boolean).map((word) => word.toLowerCase())));
  const wanted = Array.isArray(view.states) ? view.states : (required ?? []);
  const missing = wanted
    .filter((title) => !named.has(String(title).toLowerCase()))
    .map((title) => ({ uid: `${view.uid}-${title}`, title, shots: [], promised: true }));

  // Every state the model knows of is listed whether or not anything renders
  // it. Hiding the unrendered ones would hide exactly what this is for.
  return [asItIs, ...shown, ...missing];
}

/** One render, in a frame it can be scrolled inside. */
function Frame({ shot, alt, stamp, onGone }) {
  return (
    <div className="frame">
      <img
        src={`${base}img/small/${shot.file}${stamp ? `?v=${stamp}` : ""}`}
        alt={alt}
        onError={onGone}
      />
    </div>
  );
}

/**
 * The renders that belong to the theme being read in.
 *
 * A project that draws its screens both ways says so in the file name or the
 * label. One that draws them once is shown as it drew them, rather than being
 * second-guessed.
 */
function inTheme(shots, theme) {
  const other = theme === "dark" ? "light" : "dark";
  const says = (shot, word) => `${shot.label ?? ""} ${shot.file}`.toLowerCase().includes(word);
  const matching = shots.filter((shot) => says(shot, theme));
  if (matching.length > 0) return matching;
  const neutral = shots.filter((shot) => !says(shot, other));
  return neutral.length > 0 ? neutral : shots;
}

/** Every render a view carries: one screenshot, or a list of them. */
function rendersOf(view) {
  if (Array.isArray(view.renders) && view.renders.length > 0) {
    return view.renders.map((one) => (typeof one === "string" ? { file: one } : one));
  }
  return view.screenshot ? [{ file: view.screenshot }] : [];
}

/** A thumbnail, cropped when it is a tall screen and shown whole when it is wide. */
function Thumb({ view, stamp, theme }) {
  const [shape, setShape] = useState("portrait");
  const [gone, setGone] = useState(false);
  const shots = rendersOf(view);
  useEffect(() => setGone(false), [view.uid]);
  if (shots.length === 0 || gone) return <div className="noshot">nothing renders this yet</div>;
  const shot = inTheme(shots, theme)[0];
  return (
    <img
      onError={() => setGone(true)}
      className={shape}
      src={`${base}img/card/${shot.file}${stamp ? `?v=${stamp}` : ""}`}
      alt={view.title}
      loading="lazy"
      onLoad={(e) => setShape(e.target.naturalWidth > e.target.naturalHeight ? "landscape" : "portrait")}
    />
  );
}

function IndexPage({ views, model, stamp, renders, theme, required }) {
  const check = useCheck(stamp);
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
      {check.trouble?.length > 0 && (
        <div className="trouble">
          {check.trouble.map((one) => (
            <p key={one.what}>
              <strong>{one.what}</strong>
              <span>{one.why}</span>
            </p>
          ))}
        </div>
      )}
      {check.hints?.length > 0 && (
        <div className="trouble hints">
          {check.hints.map((one) => <p key={one}><strong>{one}</strong></p>)}
        </div>
      )}
      {gaps > 0 && (
        <p className="gapbanner">
          {gaps} {gaps === 1 ? "state has" : "states have"} no render. A screen is not one picture:
          it waits, it has nothing to show, and it fails. Until a project renders those, this book
          shows only the happy one.
        </p>
      )}
      {views.length === 0 && (
        <p className="hint">
          This book models no views yet. A view is a screen of the app: what it is for, what it has
          to do, and what it looks like today.
        </p>
      )}
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

/** The states, the way to compare them, and where this view is reached from. */
function Steer({ states, state, setState, view, model, extra }) {
  const reachedFrom = view.relations
    .filter((r) => r.role === "Reached from")
    .map((r) => model.views.find((v) => v.uid === r.to))
    .filter(Boolean);
  return (
    <div className="steer">
      <div className="chips">
        {states.map((one, index) => (
          <button
            key={one.uid}
            className={`${index === state ? "on" : ""} ${one.shots.length === 0 ? "gap" : ""}`}
            title={one.shots.length === 0 ? "no render of this state yet" : one.statement}
            onClick={() => setState(index)}
          >
            {one.title}
          </button>
        ))}
        {states.length > 1 && (
          <button className={state === -1 ? "on" : ""} onClick={() => setState(-1)}>
            All at once
          </button>
        )}
      </div>
      {extra}
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
      {(view.sources ?? []).length > 0 && (
        <p className="sources">
          {view.sources.map((where) => <code key={where}>{where}</code>)}
        </p>
      )}
    </div>
  );
}

function ViewPage({ model, view, onChange, stamp, theme, required, draft, onDraft, forced }) {
  const states = statesOf(model, view, required);
  const roomy = useRoomy();
  // ?showing=compare opens on every state at once, which is also how it is
  // photographed.
  const [state, setState] = useState(forced === "compare" ? -1 : 0);
  const [chosen, setChosen] = useState(0);
  const [gone, setGone] = useState({});

  const comparing = state === -1;
  // A top slice identifies a web screen; on a phone or a terminal what makes a
  // state different is often at the bottom, so the whole frame is one click away.
  const [whole, setWhole] = useState(false);
  const here = states[Math.min(Math.max(state, 0), states.length - 1)];
  // A screen drawn light and the same screen drawn dark are the same picture to
  // anyone not reading in that theme, so only the matching ones are shown.
  const shots = inTheme(here.shots, theme).filter((shot) => !gone[shot.file]);

  useEffect(() => { setState(forced === "compare" ? -1 : 0); setChosen(0); }, [view.uid, forced]);
  // A render that was missing may have been drawn since; a refresh is the moment
  // to find out rather than a reason to keep hiding it.
  useEffect(() => { setGone({}); }, [view.uid, stamp]);

  const named = (shot) => shot.label ?? shot.file.replace(/\.[a-z]+$/, "");
  const reachedFrom = view.relations
    .filter((r) => r.role === "Reached from")
    .map((r) => model.views.find((v) => v.uid === r.to))
    .filter(Boolean);

  // A window with room shows every shape at once; a phone shows one and a chip
  // to change it, because three rows of an inch each show nothing.
  const visible = roomy ? shots : shots.slice(Math.min(chosen, Math.max(shots.length - 1, 0)),
                                              Math.min(chosen, Math.max(shots.length - 1, 0)) + 1);

  // Compare lays every state side by side, one render each, which is what a
  // review is: the states next to each other rather than clicked through.
  if (comparing) {
    const shown = states
      .map((one) => ({ state: one, shot: inTheme(one.shots, theme)[0] }))
      .filter((one) => one.shot);
    return (
      <div className="page">
        <Steer
          states={states}
          state={state}
          setState={setState}
          view={view}
          model={model}
          extra={(
            <div className="chips">
              <button className={whole ? "" : "on"} onClick={() => setWhole(false)}>Tops</button>
              <button className={whole ? "on" : ""} onClick={() => setWhole(true)}>Whole</button>
            </div>
          )}
        />
        <div className={`compare ${whole ? "whole" : ""}`}>
          {shown.map(({ state: one, shot }) => (
            <figure key={one.uid}>
              <figcaption>{one.title}</figcaption>
              <img src={`${base}img/small/${shot.file}${stamp ? `?v=${stamp}` : ""}`} alt={one.title} />
            </figure>
          ))}
          {states.filter((one) => one.shots.length === 0).map((one) => (
            <figure key={one.uid} className="gap">
              <figcaption>{one.title}</figcaption>
              <div className="noshot">nothing renders this</div>
            </figure>
          ))}
        </div>
      </div>
    );
  }

  const renders = visible.map((shot) => ({
    key: shot.file,
    node: (
      <Frame
        shot={shot}
        stamp={stamp}
        alt={`${view.title}, ${here.title}, ${named(shot)}`}
        onGone={() => setGone((was) => ({ ...was, [shot.file]: true }))}
      />
    ),
  }));

  if (renders.length === 0) {
    renders.push({
      key: "none",
      node: (
        <div className="noshot tall">
          <span>
            Nothing renders {here === states[0] ? "this view" : `this view ${here.title.toLowerCase()}`} yet.
            {here.statement && <><br />{here.statement}</>}
          </span>
        </div>
      ),
    });
  }

  // What is being looked at goes with the question: which state, which render,
  // and whether the page is light or dark are half of what "this looks wrong"
  // means.
  const conversation = (
    <Ask
      key={view.uid}
      about={view.title}
      looking={[here.title, shots.map(named).join(" and "), theme].filter(Boolean).join(", ")}
      draft={draft}
      onDraft={onDraft}
      forced={forced}
    />
  );

  // Where the boundaries sit is the reader's, not the layout's: the renders and
  // the conversation are panes, dragged to whatever this screen and this
  // question call for, and remembered.
  const panes = [...renders, { key: "talk", node: conversation }];
  return (
    <div className="page">
      <Steer
        states={states}
        state={state}
        setState={setState}
        view={view}
        model={model}
        extra={!roomy && shots.length > 1 && (
          <div className="chips shapes">
            {shots.map((one, index) => (
              <button
                key={one.file}
                className={index === chosen ? "on" : ""}
                onClick={() => setChosen(index)}
              >
                {named(one)}
              </button>
            ))}
          </div>
        )}
      />
      <div className="work">
        {roomy ? (
          <Group orientation="horizontal" id={`viewbook.panes.${panes.length}`}>
            {panes.map((pane, index) => (
              <React.Fragment key={pane.key}>
                {index > 0 && <Separator className="grip" />}
                <Panel
                  minSize={10}
                  defaultSize={pane.key === "talk" ? 50 : 50 / Math.max(renders.length, 1)}
                  className="pane"
                >
                  {pane.node}
                </Panel>
              </React.Fragment>
            ))}
          </Group>
        ) : (
          <Group orientation="vertical" id={`viewbook.rows.${panes.length}`}>
            {panes.map((pane, index) => (
              <React.Fragment key={pane.key}>
                {index > 0 && <Separator className="grip across" />}
                <Panel minSize={10} defaultSize={pane.key === "talk" ? 50 : 50 / Math.max(renders.length, 1)} className="pane">
                  {pane.node}
                </Panel>
              </React.Fragment>
            ))}
          </Group>
        )}
      </div>

    </div>
  );
}

/**
 * Say something to the session, and see it land.
 *
 * The page had a notes box that saved silently, so asking for something looked
 * exactly like doing nothing: no button, no confirmation, no reply. This is the
 * other half of the loop - what is typed goes to the conversation, and what the
 * conversation says comes back underneath.
 */
function Ask({ about, looking, draft, onDraft, forced }) {
  const text = draft ?? "";
  const setText = onDraft ?? (() => {});
  // ?showing=attached holds the state where an image is waiting to go with the
  // message, which is otherwise a state only a paste can reach.
  const [shots, setShots] = useState(() =>
    (forced === "attached" ? [{ url: "img/small/index-tall-dark.png", path: "(pasted image)" }] : []));
  const [state, setState] = useState("");
  const [reply, setReply] = useState("");
  const [busy, setBusy] = useState(false);
  const [running, setRunning] = useState(true);
  const [canWake, setCanWake] = useState(false);
  const talk = useRef(null);


  // The conversation is there whether or not this page asked the last question,
  // so it is shown on arrival rather than only after sending something.


  // A session that is mid-answer cannot be typed into. Waiting a few seconds and
  // trying again is what a person would do, so the page does it rather than
  // handing back a number.
  const sendOnce = (said, tries) => api("api/say", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text: said }),
  })
    .then(() => {
      setState("");
      setText("");
      setShots([]);
      setBusy(false);
    })
    .catch((error) => {
      if (tries > 0 && /could not type|busy/i.test(error.message)) {
        setState("the session is busy, trying again…");
        setTimeout(() => sendOnce(said, tries - 1), 4000);
        return;
      }
      setState(`not sent: ${error.message}`);
      setBusy(false);
    });

  const send = () => {
    const message = text.trim();
    if (busy || (!message && shots.length === 0)) return;
    setBusy(true);
    setState("");
    // A pasted image is already a file in the project; the conversation is given
    // its path, which is something it can actually open.
    const attached = shots.map((s) => `\n${s.path}`).join("");
    // Which render is on screen, and whether it is light or dark, travels with
    // the message: otherwise the answer is about a picture nobody is looking at.
    //
    // A command is not a question about the view. It goes as typed, because
    // "About A view: /doner off" is prose, and /doner off is a command.
    const command = message.startsWith("/");
    const at = about && !command ? `About ${about}${looking ? ` (${looking})` : ""}: ` : "";
    const said = at + message + attached;
    sendOnce(said, 3);
  };

  const paste = (event) => {
    const images = [...(event.clipboardData?.items ?? [])]
      .filter((item) => item.type.startsWith("image/"));
    if (images.length === 0) return;
    event.preventDefault();
    setState("");
    images.forEach((item) => {
      const file = item.getAsFile();
      if (!file) return;
      fetch(base + "api/paste", {
        method: "POST",
        headers: { "Content-Type": file.type },
        body: file,
      })
        .then((r) => r.json())
        .then((kept) => setShots((now) => [...now, kept]))
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
    const read = () => api("api/session")
      .then((r) => { setReply(r.text || ""); setRunning(Boolean(r.running)); setCanWake(Boolean(r.canWake)); })
      .catch(() => {});
    read();
    const every = setInterval(read, 2500);
    return () => clearInterval(every);
  }, []);

  // The conversation stands beside the render, because those two and what is
  // typed between them are what the page is for.
  return (
    <section className="talk">
      {canWake && (
        <div className="ask-bar session">
          <span>{running ? "session running" : "no session"}</span>
          <button
            className="quiet"
            onClick={() => api("api/session", { method: running ? "DELETE" : "POST" })
              .then((r) => setRunning(Boolean(r.running)))
              .catch((error) => setState(error.message))}
          >
            {running ? "stop it" : "start it"}
          </button>
        </div>
      )}
      {reply
        ? <pre className="reply" ref={talk}>{reply.trimEnd().split("\n").slice(-80).join("\n")}</pre>
        : <p className="hint">Nothing in the session yet.</p>}
      {shots.length > 0 && (
        <div className="shots">
          {shots.map((shot) => (
            <span key={shot.url}>
              <img src={base + shot.url} alt="attached" />
              <button className="quiet" onClick={() => setShots((now) => now.filter((s) => s !== shot))}>
                remove
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="dock-row">
      <textarea
        className="ask"
        rows={1}
        value={text}
        placeholder={`Ask about ${about ?? "this"}`}
        onChange={(e) => setText(e.target.value)}
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
        <button
          className="send"
          onClick={send}
          disabled={busy || (!text.trim() && shots.length === 0)}
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
