import java.io.File;
import java.io.FileOutputStream;
import java.util.Properties;
import org.apache.maven.AbstractMavenLifecycleParticipant;
import org.apache.maven.MavenExecutionException;
import org.apache.maven.execution.MavenSession;
import org.apache.maven.project.MavenProject;
import org.apache.maven.model.Dependency;
import org.eclipse.aether.repository.RemoteRepository;

// Runs after model inheritance/profile/settings processing, before build execution.
public final class MavenSources extends AbstractMavenLifecycleParticipant {
    @Override public void afterProjectsRead(MavenSession session) throws MavenExecutionException {
        Properties report = new Properties();
        boolean declared = false;
        boolean unsupported = false;
        int index = 0;
        String requested = System.getenv("DEPTRUST_SOURCE_PACKAGE");
        for (MavenProject project : session.getProjects()) {
            if ((project.getGroupId() + ":" + project.getArtifactId()).equals(requested)) unsupported = true;
            boolean matches = false;
            for (Dependency dependency : project.getDependencies()) {
                if ((dependency.getGroupId() + ":" + dependency.getArtifactId()).equals(requested)) {
                    matches = true;
                    declared = true;
                    if ("system".equals(dependency.getScope()) || dependency.getSystemPath() != null) unsupported = true;
                }
            }
            if (!matches) continue;
            for (RemoteRepository repository : project.getRemoteProjectRepositories()) {
                RemoteRepository mirror = session.getRepositorySession().getMirrorSelector().getMirror(repository);
                report.setProperty("source." + index++, (mirror == null ? repository : mirror).getUrl());
            }
        }
        report.setProperty("declared", Boolean.toString(declared && !unsupported));
        report.setProperty("complete", "true");
        try (FileOutputStream stream = new FileOutputStream(System.getenv("DEPTRUST_SOURCE_OUTPUT"))) {
            report.storeToXML(stream, "DepTrust effective Maven source report", "UTF-8");
        } catch (Exception error) {
            throw new MavenExecutionException("Cannot write source report", error);
        }
        // Intentional stop before lifecycle goals or dependency resolution run.
        throw new MavenExecutionException("DEPTRUST_SOURCE_INSPECTED", (File) null);
    }
}
