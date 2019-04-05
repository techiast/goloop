package foundation.icon.test.suites;

import foundation.icon.icx.KeyWallet;
import foundation.icon.test.cases.BasicScoreTest;
import foundation.icon.test.cases.MultiSigWalletTest;
import foundation.icon.test.cases.RevertTest;
import foundation.icon.test.common.Env;
import foundation.icon.test.common.Utils;
import org.junit.AfterClass;
import org.junit.BeforeClass;
import org.junit.runner.RunWith;
import org.junit.runners.Suite;

import java.io.File;
import java.io.IOException;
import java.math.BigInteger;
import java.util.Map;
import java.util.concurrent.TimeUnit;

@RunWith(Suite.class)
@Suite.SuiteClasses({
        BasicScoreTest.class,
        MultiSigWalletTest.class,
        RevertTest.class
})
public class SingleChainTestSuite {
    @BeforeClass
    public static void setUp() throws Exception {
        startGoLoop();

        KeyWallet god = Utils.readWalletFromFile("./data/keystore_god.json", "gochain");
        Env.Chain chain = new Env.Chain(BigInteger.valueOf(3), god);
        Env.Node node = new Env.Node("http://localhost:9080/api/v3", new Env.Chain[]{chain});
        Env.nodes = new Env.Node[]{node};

        Env.LOG.setLevel(Config.TEST_LOG_LEVEL);
    }

    @AfterClass
    public static void tearDown() {
        stopGoLoop();
    }

    // TODO Share the following methods in common class?
    private static Process goLoop;

    public static void startGoLoop() {
        try {
            Runtime.getRuntime().exec("rm -rf .chain");

            // TODO Make it configurable
            // TODO Consider how to print log (care for it later with docker)
            // TODO Get god wallet from config.json, not from additional file.
            ProcessBuilder pb = new ProcessBuilder("../bin/gochain", "-config=./data/config.json");
            Map<String, String> env = pb.environment();
            // TODO how to handle with virtual env
            String separator = System.getProperties().getProperty("path.separator");
            env.put("PATH", "../.venv/bin" + separator + env.get("PATH"));
            env.put("PYTHONPATH", "../pyee");
            pb.directory(new File("."));

            if (Config.WITH_NODE_LOG) {
                pb.redirectError(ProcessBuilder.Redirect.INHERIT);
                pb.redirectOutput(ProcessBuilder.Redirect.INHERIT);
            }

            goLoop = pb.start();

            Thread.sleep(3000);
        } catch (IOException | InterruptedException ex) {
            ex.printStackTrace();
        }
    }

    public static void stopGoLoop() {
        try {
            goLoop.destroy();
            goLoop.getErrorStream().close();
            goLoop.getInputStream().close();
            goLoop.getOutputStream().close();
            goLoop.waitFor(5, TimeUnit.SECONDS);

            Env.LOG.info("Sub process is killed");
        }
        catch (Exception e) {
            e.printStackTrace();
        }
    }
}
